package reconciliation

import (
	"context"
	"errors"
	"oci-maintenance-router/internal/delivery"
	"testing"
	"time"
)

func TestMaintenancePinnedVersionFailureNeverFallsBackToCurrent(t *testing.T) {
	for _, kind := range []string{"unavailable", "nil", "wrong-version", "invalid-key"} {
		n := processingNotice(t, 1, processingDB)
		f := processingFixtureFor(t, n)
		f.seedBatch(t, n, time.Now().Add(-time.Minute), delivery.Prepared, false)
		f.p.version = func(_ context.Context, id string, version int64) (maintenanceCredential, error) {
			if id != "ocid1.vaultsecret.oc1.iad.payments" || version != 7 {
				t.Fatal("saved secret identity changed")
			}
			switch kind {
			case "unavailable":
				return nil, errors.New("private secret detail")
			case "nil":
				return nil, nil
			case "wrong-version":
				return diagnosticMaterial{8, func() []byte { return make([]byte, 32) }}, nil
			default:
				return diagnosticMaterial{7, func() []byte { return []byte("bad") }}, nil
			}
		}
		s := f.run(t)
		b := f.saved(t, n.Key().NoticeID).Snapshot().Batches[0]
		if s.Accepted != 0 || len(s.Issues) != 1 || b.State != delivery.ReviewRequired || b.Reason != delivery.ReasonSecretUnavailable || f.current.Load() != 0 || f.sent.Load() != 0 {
			t.Fatal("pinned version failure fell back or disappeared")
		}
	}
}

func TestMaintenanceCurrentSecretFailureLeavesResourcesUnassigned(t *testing.T) {
	for _, kind := range []string{"unavailable", "nil", "nonpositive"} {
		n := processingNotice(t, 1, processingDB)
		f := processingFixtureFor(t, n)
		f.p.current = func(context.Context, string) (maintenanceCredential, error) {
			switch kind {
			case "unavailable":
				return nil, errors.New("private secret detail")
			case "nil":
				return nil, nil
			default:
				return diagnosticMaterial{0, func() []byte { return make([]byte, 32) }}, nil
			}
		}
		s := f.run(t)
		record := f.saved(t, n.Key().NoticeID).Snapshot()
		if s.Prepared != 0 || s.Accepted != 0 || len(s.Issues) != 1 || len(record.Batches) != 0 || record.Revisions[0].Resources[0].BatchID != "" {
			t.Fatal("failed secret was pinned/assigned")
		}
	}
}

func TestMaintenanceRotationDoesNotChangeFirstSavedVersion(t *testing.T) {
	n := processingNotice(t, 1, processingDB)
	f := processingFixtureFor(t, n)
	old := f.seedBatch(t, n, time.Now().Add(-time.Minute), delivery.Prepared, false)
	f.p.current = func(context.Context, string) (maintenanceCredential, error) {
		t.Fatal("saved work reread CURRENT")
		return nil, nil
	}
	s := f.run(t)
	b := f.saved(t, n.Key().NoticeID).Snapshot().Batches[0]
	if s.Accepted != 1 || b.SecretVersion != 7 || b.Message.PayloadDigest != old.Snapshot().Batches[0].Message.PayloadDigest {
		t.Fatal("rotation changed saved identity")
	}
}
