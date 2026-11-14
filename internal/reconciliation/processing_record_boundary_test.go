package reconciliation

import (
	"context"
	"oci-maintenance-router/internal/delivery"
	"oci-maintenance-router/internal/ocistorage"
	"testing"
	"time"
)

func TestMaintenanceRejectsStateChangesAfterFinalDiscovery(t *testing.T) {
	for _, kind := range []string{"missing", "membership", "configuration", "clock", "recovery_clock", "recovery_write"} {
		n := processingNotice(t, 1, processingDB)
		f := processingFixtureFor(t, n)
		if kind == "recovery_write" {
			f.seedBatch(t, n, time.Now().Add(-2*time.Minute), delivery.Sending, false)
		}
		clock := f.p.discovery.now
		reads := 0
		completed := false
		f.p.discovery.now = func() time.Time {
			at := clock()
			reads++
			// One observation time and one completion time in each actual pass.
			if reads == 4 {
				completed = true
				switch kind {
				case "missing":
					f.live.objects.mu.Lock()
					delete(f.live.objects.objects, noticeName(n.Key().NoticeID))
					f.live.objects.mu.Unlock()
				case "membership":
					old := f.saved(t, n.Key().NoticeID)
					v := n.Snapshot()
					v.Reference = chain("unexpected revision")
					next, err := f.p.observer.Observe(old, normalizeCandidate(t, v), true, nil, at)
					if err != nil {
						t.Fatal(err)
					}
					body, _ := next.Encode()
					f.live.objects.seed(noticeName(n.Key().NoticeID), body)
				case "configuration":
					f.live.objects.seed("configuration/router.json", []byte("changed"))
				case "clock":
					f.live.lease.manager.now = func() time.Time { return time.Time{} }
				}
			}
			return at
		}
		f.live.objects.after = func(_ context.Context, c discoveryCall) error {
			if completed && kind == "recovery_clock" && c.method == "GET" && c.name == noticeName(n.Key().NoticeID) {
				f.live.lease.manager.now = func() time.Time { return time.Unix(1, 0) }
			}
			return nil
		}
		f.live.objects.before = func(_ context.Context, c discoveryCall) error {
			if completed && kind == "recovery_write" && c.method == "REPLACE" && c.name == noticeName(n.Key().NoticeID) {
				return &ocistorage.Error{Code: ocistorage.Service, Status: 403}
			}
			return nil
		}
		r, err := f.p.Run(leaseBudget(t), f.live.lease)
		if !completed || err == nil || r.Snapshot().Completed || f.sent.Load() != 0 {
			t.Fatal("changed state/authority used after discovery", kind, err)
		}
	}
}

func TestMaintenanceInvalidClockBetweenDiscoveryAndDeliveryStopsActivation(t *testing.T) {
	f := processingFixtureFor(t)
	f.p.discovery.now = func() time.Time {
		at := time.Now()
		f.live.lease.manager.now = func() time.Time { return time.Time{} }
		return at
	}
	r, err := f.p.Run(leaseBudget(t), f.live.lease)
	if err == nil || r.Snapshot().Completed || f.sent.Load() != 0 {
		t.Fatal("invalid phase-start clock accepted")
	}
}

func TestMaintenanceMissingReadinessPreventsFinalScanAndDelivery(t *testing.T) {
	f := processingFixtureFor(t, processingNotice(t, 1, processingDB))
	f.live.objects.seed(diagnosticObjectName, []byte("invalid receipt"))
	r, err := f.p.Run(leaseBudget(t), f.live.lease)
	if err == nil || r.Snapshot().Completed || f.listed.Load() != 1 || f.sent.Load() != 0 {
		t.Fatal("missing readiness authorized processing")
	}
}
