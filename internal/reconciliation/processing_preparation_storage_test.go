package reconciliation

import (
	"context"
	"oci-maintenance-router/internal/delivery"
	"oci-maintenance-router/internal/ocistorage"
	"testing"
	"time"
)

func TestMaintenanceFailedPreparationSaveLeavesNoAssignmentOrDispatch(t *testing.T) {
	n := processingNotice(t, 1, processingDB)
	f := processingFixtureFor(t, n)
	f.live.objects.before = func(_ context.Context, call discoveryCall) error {
		if processingWriteState(t, f, call) == delivery.Prepared {
			return &ocistorage.Error{Code: ocistorage.Service, Status: 403}
		}
		return nil
	}
	r, err := f.p.Run(leaseBudget(t), f.live.lease)
	if err == nil || r.Snapshot().Completed || r.Snapshot().Prepared != 0 || f.sent.Load() != 0 {
		t.Fatal("failed preparation was reported or sent", err)
	}
	saved := f.saved(t, n.Key().NoticeID).Snapshot()
	if len(saved.Batches) != 0 || saved.Revisions[0].Resources[0].BatchID != "" {
		t.Fatal("failed preparation left an assignment")
	}
}

func TestMaintenanceClockRollbackDuringCurrentReadCannotPinVersion(t *testing.T) {
	n := processingNotice(t, 1, processingDB)
	f := processingFixtureFor(t, n)
	current := f.p.current
	f.p.current = func(ctx context.Context, id string) (maintenanceCredential, error) {
		credential, err := current(ctx, id)
		f.live.lease.manager.now = func() time.Time { return time.Unix(1, 0) }
		return credential, err
	}
	r, err := f.p.Run(leaseBudget(t), f.live.lease)
	if err == nil || r.Snapshot().Completed || r.Snapshot().Prepared != 0 || f.sent.Load() != 0 {
		t.Fatal("rollback permitted immutable preparation", err)
	}
	if len(f.saved(t, n.Key().NoticeID).Snapshot().Batches) != 0 {
		t.Fatal("invalid preparation pinned a version")
	}
}

func TestMaintenanceFailedSecretHoldPreservesPendingBatch(t *testing.T) {
	for _, failure := range []string{"clock", "storage"} {
		t.Run(failure, func(t *testing.T) {
			n := processingNotice(t, 1, processingDB)
			f := processingFixtureFor(t, n)
			f.seedBatch(t, n, time.Now().Add(-time.Minute), delivery.Prepared, false)
			f.p.version = func(context.Context, string, int64) (maintenanceCredential, error) {
				if failure == "clock" {
					f.live.lease.manager.now = func() time.Time { return time.Unix(1, 0) }
				}
				return nil, nil
			}
			f.live.objects.before = func(_ context.Context, call discoveryCall) error {
				if failure == "storage" && processingWriteState(t, f, call) == delivery.ReviewRequired {
					return &ocistorage.Error{Code: ocistorage.Service, Status: 403}
				}
				return nil
			}
			r, err := f.p.Run(leaseBudget(t), f.live.lease)
			if err == nil || r.Snapshot().Completed || len(r.Snapshot().Issues) != 0 || f.sent.Load() != 0 {
				t.Fatal("failed hold was reported as saved or dispatched", err)
			}
			if f.saved(t, n.Key().NoticeID).Snapshot().Batches[0].State != delivery.Prepared {
				t.Fatal("failed hold overwrote pending state")
			}
		})
	}
}
