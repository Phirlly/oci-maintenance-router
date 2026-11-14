package reconciliation

import (
	"context"
	"oci-maintenance-router/internal/delivery"
	"oci-maintenance-router/internal/notices"
	"oci-maintenance-router/internal/routing"
	"testing"
)

func TestMaintenanceProcessesNewCurrentOwnerOnceAndRestarts(t *testing.T) {
	n := processingNotice(t, 1, processingDB)
	f := processingFixtureFor(t, n)
	s := f.run(t)
	if !s.Completed || s.Prepared != 1 || s.Accepted != 1 || len(s.Attempts) != 1 || s.Attempts[0].Status != 202 || !s.Attempts[0].Confirmed || len(s.Issues) != 0 {
		t.Fatalf("wrong result: %+v", s)
	}
	if f.listed.Load() != 2 || f.fetched.Load() < 4 || f.current.Load() != 1 || f.exact.Load() != 1 || f.sent.Load() != 1 || f.live.channelReads.Load() < 2 {
		t.Fatal("required fresh reads/claims missing")
	}
	b := f.saved(t, n.Key().NoticeID).Snapshot().Batches[0]
	if b.State != delivery.APIAccepted || b.AttemptCount != 1 || b.SecretVersion != 7 {
		t.Fatal("acceptance not durable")
	}
	again := f.run(t)
	if !again.Completed || again.Accepted != 0 || again.Prepared != 0 || f.sent.Load() != 1 || f.current.Load() != 1 {
		t.Fatal("accepted work replayed")
	}
}

func TestMaintenanceRetainsUnresolvedOwnershipWithoutPreparing(t *testing.T) {
	n := processingNotice(t, 1, processingDB)
	f := processingFixtureFor(t, n)
	f.p.mysql["us-ashburn-1"] = func(_ context.Context, id string) (routing.Metadata, error) {
		return routing.Metadata{ID: chain(id), CompartmentID: chain("ocid1.compartment.oc1..example")}, nil
	}
	s := f.run(t)
	if !s.Completed || s.Accepted != 0 || len(s.Issues) == 0 || f.current.Load() != 0 || f.sent.Load() != 0 {
		t.Fatal("unresolved work hidden or sent")
	}
	row := f.saved(t, n.Key().NoticeID).Snapshot().Revisions[0].Resources[0]
	if row.Disposition != "UNRESOLVED" || row.Reason != "tag_missing" {
		t.Fatal("unresolved ownership not retained")
	}
}

func TestMaintenanceDoesNotPinMalformedCurrentKey(t *testing.T) {
	n := processingNotice(t, 1, processingDB)
	f := processingFixtureFor(t, n)
	f.p.current = func(context.Context, string) (maintenanceCredential, error) {
		return diagnosticMaterial{8, func() []byte { return []byte("bad") }}, nil
	}
	s := f.run(t)
	if !s.Completed || len(s.Issues) == 0 || s.Prepared != 0 || f.sent.Load() != 0 || len(f.saved(t, n.Key().NoticeID).Snapshot().Batches) != 0 {
		t.Fatal("malformed key became immutable work")
	}
}

func TestMaintenancePersistsLateSourceChangeAndStopsSending(t *testing.T) {
	n := processingNotice(t, 1, processingDB)
	f := processingFixtureFor(t, n)
	get := f.source.get
	f.source.get = func(ctx context.Context, id string) (notices.Notice, error) {
		got, err := get(ctx, id)
		if f.fetched.Load() >= 4 {
			r := got.Snapshot()
			r.Reference = chain("changed")
			return normalizeCandidate(t, r), nil
		}
		return got, err
	}
	f.p.discovery.collector.announcements = f.source
	r, err := f.p.Run(leaseBudget(t), f.live.lease)
	if err == nil || r.Snapshot().Completed || f.sent.Load() != 0 {
		t.Fatal("late changed source sent or claimed complete")
	}
	s := f.saved(t, n.Key().NoticeID).Snapshot()
	if len(s.Revisions) != 2 || s.HoldReason != delivery.ReasonSourceChanged {
		t.Fatal("late observation/hold lost")
	}
}
