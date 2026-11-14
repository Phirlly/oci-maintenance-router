package reconciliation

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	"oci-maintenance-router/internal/noticeledger"
	"oci-maintenance-router/internal/notices"
	"oci-maintenance-router/internal/ocistorage"
)

func TestDiscoverySavedReloadFailureCannotInferAbsence(t *testing.T) {
	n := candidateNotice(t, 1, nil, true)
	d, s, lease := discoveryFixture(t, []notices.Notice{n}, []noticeledger.Record{candidateRecord(t, n)})
	reads := 0
	s.before = func(_ context.Context, call discoveryCall) error {
		if call.method == "GET" && call.name == noticeName(n.Key().NoticeID) {
			reads++
			if reads == 2 {
				return &ocistorage.Error{Code: ocistorage.Service, Status: 404}
			}
		}
		return nil
	}
	got, err := d.Run(leaseBudget(t), lease)
	discoveryFailure(t, got, err, "record_read")
	if len(s.noticeWrites()) != 0 {
		t.Fatal("missing saved record recreated")
	}
}

func TestDiscoveryExplicitWriteRejectionsNeverConfirmOrRetry(t *testing.T) {
	for _, status := range []int{400, 401, 403, 404, 408, 409, 429} {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			n := candidateNotice(t, 1, nil, true)
			d, s, lease := discoveryFixture(t, []notices.Notice{n}, nil)
			s.before = func(_ context.Context, call discoveryCall) error {
				if call.method == "CREATE" {
					return &ocistorage.Error{Code: ocistorage.Service, Status: status, WriteUncertain: true}
				}
				return nil
			}
			got, err := d.Run(leaseBudget(t), lease)
			discoveryFailure(t, got, err, "write_failed")
			if len(s.noticeWrites()) != 1 {
				t.Fatal("rejection retried")
			}
			for _, call := range s.calls {
				if call.method == "GET" && strings.HasPrefix(call.name, "notices/") {
					t.Fatal("explicit rejection confirmed")
				}
			}
		})
	}
}

func TestDiscoveryFailedConfirmationRemainsIncomplete(t *testing.T) {
	n := candidateNotice(t, 1, nil, true)
	d, s, lease := discoveryFixture(t, []notices.Notice{n}, nil)
	s.after = func(_ context.Context, call discoveryCall) error {
		if call.method == "CREATE" {
			return errors.New("private transport detail")
		}
		return nil
	}
	s.before = func(_ context.Context, call discoveryCall) error {
		if call.method == "GET" && strings.HasPrefix(call.name, "notices/") {
			return &ocistorage.Error{Code: ocistorage.Service, Status: 503}
		}
		return nil
	}
	got, err := d.Run(leaseBudget(t), lease)
	discoveryFailure(t, got, err, "confirmation_read")
	if len(s.noticeWrites()) != 1 || strings.Contains(err.Error(), "private") {
		t.Fatal("uncertain failure retried or leaked")
	}
}

func TestDiscoveryTransitionClockFailureStopsAtLastConfirmedParent(t *testing.T) {
	for _, stage := range []string{"new", "migration", "unavailable", "hold"} {
		t.Run(stage, func(t *testing.T) {
			n := candidateNotice(t, 1, chain("shared"), true)
			ns := []notices.Notice{n}
			var records []noticeledger.Record
			if stage == "migration" {
				records = []noticeledger.Record{discoveryLegacy(t, candidateRecord(t, n))}
			}
			if stage == "unavailable" {
				ns = nil
				records = []noticeledger.Record{candidateRecord(t, n)}
			}
			if stage == "hold" {
				ns = append(ns, candidateNotice(t, 2, chain("shared"), true))
			}
			d, s, lease := discoveryFixture(t, ns, records)
			calls := 0
			clock := d.now
			d.now = func() time.Time {
				calls++
				if stage == "hold" && calls == 1 {
					return clock()
				}
				return time.Time{}
			}
			got, err := d.Run(leaseBudget(t), lease)
			discoveryFailure(t, got, err, "transition_failed")
			want := 0
			if stage == "hold" {
				want = 1
			}
			if len(s.noticeWrites()) != want {
				t.Fatal("failed transition wrote or discarded prior work")
			}
		})
	}
}

func TestDiscoveryRejectsInvalidCompletionClockAndZeroReceiver(t *testing.T) {
	d, _, lease := discoveryFixture(t, nil, nil)
	d.now = func() time.Time { return time.Time{} }
	got, err := d.Run(leaseBudget(t), lease)
	discoveryFailure(t, got, err, "clock_invalid")
	got, err = (*Discovery)(nil).Run(leaseBudget(t), lease)
	discoveryFailure(t, got, err, "invalid_request")
}
