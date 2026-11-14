package reconciliation

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"oci-maintenance-router/internal/delivery"
	"oci-maintenance-router/internal/noticeledger"
	"oci-maintenance-router/internal/notices"
	"oci-maintenance-router/internal/ocistorage"
	"oci-maintenance-router/internal/routing"
)

func heldPreparationRequest(t *testing.T, f *processingFixture, n notices.Notice) {
	t.Helper()
	lookups := map[string]routing.Lookup{}
	_, targets := f.p.policy.Targets(n, true)
	for _, target := range targets {
		m, err := f.p.mysql[target.Region](leaseBudget(t), target.ResourceID)
		if err != nil {
			t.Fatal(err)
		}
		lookups[target.ResourceID] = routing.Lookup{State: routing.Found, Metadata: m}
	}
	r, err := f.p.observer.Observe(noticeledger.Record{}, n, true, lookups, time.Now())
	if err == nil {
		r, err = r.Hold(delivery.ReasonOperator, time.Now())
	}
	if err != nil {
		t.Fatal(err)
	}
	body, _ := r.Encode()
	f.live.objects.seed(noticeName(n.Key().NoticeID), body)
	q := reviewedRecovery(t, r, noticeledger.ApproveRevision, n.Key().Digest)
	f.configure(t, func(v map[string]any) { v["schemaVersion"], v["recovery"] = 7, q })
}

func TestHeldApprovalRetainsPayloadAndAtomicCapacityFailures(t *testing.T) {
	for _, capacity := range []bool{false, true} {
		t.Run(map[bool]string{false: "payload", true: "state"}[capacity], func(t *testing.T) {
			ids := []string{processingDB, "ocid1.mysqldbsystem.oc1.iad." + strings.Repeat("x", 40000)}
			if capacity {
				ids = make([]string, 256)
				for i := range ids {
					ids[i] = fmt.Sprintf("ocid1.mysqldbsystem.oc1.iad.%03d%s", i, strings.Repeat("x", 3300))
				}
			}
			n := processingNotice(t, 1, ids...)
			f := processingFixtureFor(t, n)
			heldPreparationRequest(t, f, n)
			r, err := f.p.Run(leaseBudget(t), f.live.lease)
			if capacity {
				if e, ok := err.(*ProcessingError); !ok || e.Code != "recovery_failed" || r.Snapshot().Completed {
					t.Fatal("capacity changed approval outcome", err)
				}
			} else if err != nil || r.Snapshot().Recovery == nil || r.Snapshot().Recovery.Reason != "no_eligible_work" {
				t.Fatal("payload changed rejection", err)
			}
			p := savedPreparationIssues(t, f, n.Key().NoticeID).Snapshot()
			want, code := 1, "payload_too_large"
			if capacity {
				want, code = 256, "state_capacity"
			}
			if len(p.Observations) != want || p.Observations[0].Code != code || f.sent.Load() != 0 || len(f.saved(t, n.Key().NoticeID).Snapshot().Batches) != 0 {
				t.Fatal("atomic preparation failure lost or partially prepared", len(p.Observations))
			}
		})
	}
}

func TestHeldApprovalIssueWriteFailureStopsBeforeRetainingRejection(t *testing.T) {
	for _, payload := range []bool{false, true} {
		id := processingDB
		if payload {
			id = "ocid1.mysqldbsystem.oc1.iad." + strings.Repeat("x", 40000)
		}
		n := processingNotice(t, 1, id)
		f := processingFixtureFor(t, n)
		heldPreparationRequest(t, f, n)
		f.p.current = func(context.Context, string) (maintenanceCredential, error) { return nil, nil }
		f.live.objects.before = func(_ context.Context, c discoveryCall) error {
			if strings.HasPrefix(c.name, "review/preparation/") && c.method == "CREATE" {
				return &ocistorage.Error{Code: ocistorage.Service, Status: 403}
			}
			return nil
		}
		r, err := f.p.Run(leaseBudget(t), f.live.lease)
		if err == nil || r.Snapshot().Completed || f.sent.Load() != 0 || len(recoveryActions(t, f, n.Key().NoticeID)) != 0 {
			t.Fatal("failed evidence write consumed approval", err)
		}
	}
}
