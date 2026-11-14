package reconciliation

import (
	"context"
	"testing"

	"oci-maintenance-router/internal/noticeledger"
	"oci-maintenance-router/internal/notices"
	"oci-maintenance-router/internal/ocistorage"
)

func TestDiscoveryExactUncertainWritesSupplyFreshParents(t *testing.T) {
	n := candidateNotice(t, 1, nil, true)
	for _, saved := range []bool{false, true} {
		t.Run(map[bool]string{false: "create", true: "legacy"}[saved], func(t *testing.T) {
			var records []noticeledger.Record
			if saved {
				records = []noticeledger.Record{discoveryLegacy(t, candidateRecord(t, n))}
			}
			d, s, lease := discoveryFixture(t, []notices.Notice{n, n}, records)
			s.after = func(_ context.Context, call discoveryCall) error {
				if call.method == "CREATE" || call.method == "REPLACE" {
					return &ocistorage.Error{Code: ocistorage.Transport, WriteUncertain: true}
				}
				return nil
			}
			got, err := d.Run(leaseBudget(t), lease)
			if err != nil {
				t.Fatal("exact confirmation failed", err)
			}
			want := 2
			if saved {
				want = 3
			}
			if len(s.noticeWrites()) != want {
				t.Fatal("write was retried or transition skipped")
			}
			if s.record(t, got.EnvironmentID(), n.Key().NoticeID).Snapshot().SourceOutcome.State != noticeledger.SourceAvailable {
				t.Fatal("observation not durable")
			}
		})
	}
}

func TestDiscoveryConflictNeverMergesUnobservedRecord(t *testing.T) {
	n := candidateNotice(t, 1, nil, true)
	d, s, lease := discoveryFixture(t, []notices.Notice{n}, nil)
	s.before = func(_ context.Context, call discoveryCall) error {
		if call.method == "CREATE" {
			old := discoveryLegacy(t, candidateRecord(t, n))
			body, err := old.Encode()
			if err != nil {
				t.Fatal(err)
			}
			s.seed(call.name, body)
		}
		return nil
	}
	got, err := d.Run(leaseBudget(t), lease)
	discoveryFailure(t, got, err, "conflict")
	if len(s.noticeWrites()) != 1 || s.record(t, lease.manager.environmentID, n.Key().NoticeID).Snapshot().SchemaVersion != 1 {
		t.Fatal("conflicting record changed")
	}
}

func TestDiscoveryUncertainMismatchFailsWithoutRetry(t *testing.T) {
	n := candidateNotice(t, 1, nil, true)
	d, s, lease := discoveryFixture(t, []notices.Notice{n}, nil)
	s.after = func(_ context.Context, call discoveryCall) error {
		if call.method == "CREATE" {
			old := discoveryLegacy(t, candidateRecord(t, n))
			body, err := old.Encode()
			if err != nil {
				t.Fatal(err)
			}
			s.seed(call.name, body)
			return &ocistorage.Error{Code: ocistorage.Transport, WriteUncertain: true}
		}
		return nil
	}
	got, err := d.Run(leaseBudget(t), lease)
	discoveryFailure(t, got, err, "confirmation_mismatch")
	if len(s.noticeWrites()) != 1 {
		t.Fatal("uncertain write retried")
	}
}
