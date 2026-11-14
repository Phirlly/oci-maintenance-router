package reconciliation

import (
	"context"
	"oci-maintenance-router/internal/noticeledger"
	"oci-maintenance-router/internal/notices"
	"oci-maintenance-router/internal/routing"
	"testing"
	"time"
)

func TestRecoveryApprovalDoesNotIncludeLaterResolvedBaselineRows(t *testing.T) {
	other := "ocid1.mysqldbsystem.oc1.iad.later"
	n := processingNotice(t, 1, processingDB, other)
	f := processingFixtureWithBaseline(t, []notices.Notice{n}, n)
	lookup, resolved := f.p.mysql["us-ashburn-1"], false
	f.p.mysql["us-ashburn-1"] = func(ctx context.Context, id string) (routing.Metadata, error) {
		m, err := lookup(ctx, id)
		if id == other && !resolved {
			m.FreeformTags = nil
		}
		return m, err
	}
	lookups := map[string]routing.Lookup{}
	for _, id := range []string{processingDB, other} {
		m, err := f.p.mysql["us-ashburn-1"](leaseBudget(t), id)
		if err != nil {
			t.Fatal(err)
		}
		lookups[id] = routing.Lookup{State: routing.Found, Metadata: m}
	}
	old, err := f.p.observer.Observe(noticeledger.Record{}, n, true, lookups, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	body, _ := old.Encode()
	f.live.objects.seed(noticeName(n.Key().NoticeID), body)
	q := reviewedRecovery(t, old, noticeledger.ApproveRevision, n.Key().Digest)
	f.configure(t, func(v map[string]any) { v["schemaVersion"], v["recovery"] = 7, q })
	s := f.run(t)
	if s.Accepted != 1 || f.sent.Load() != 1 {
		t.Fatal("eligible reviewed work did not send", s)
	}
	resolved = true
	f.run(t)
	after := f.saved(t, n.Key().NoticeID).Snapshot()
	if f.sent.Load() != 1 || len(after.Batches) != 1 || len(after.Batches[0].Message.Resources) != 1 || len(after.Recovery.Actions[0].BatchIDs) != 1 {
		t.Fatal("approval expanded to later-resolved work")
	}
}

func TestRecoveryApprovalSecretFailureCannotSavePartialPreparation(t *testing.T) {
	other := "ocid1.mysqldbsystem.oc1.iad.video"
	n := processingNotice(t, 1, processingDB, other)
	f := processingFixtureWithBaseline(t, []notices.Notice{n}, n)
	edit := func(v map[string]any) {
		v["destinations"] = append(v["destinations"].([]any), map[string]any{"id": "video", "secretId": "ocid1.vaultsecret.oc1.iad.video"})
		v["routes"] = append(v["routes"].([]any), map[string]any{"ownerValue": "video", "destinationId": "video"})
	}
	f.configure(t, edit)
	lookup := f.p.mysql["us-ashburn-1"]
	f.p.mysql["us-ashburn-1"] = func(ctx context.Context, id string) (routing.Metadata, error) {
		m, err := lookup(ctx, id)
		if id == other {
			m.FreeformTags["owner"] = "video"
		}
		return m, err
	}
	lookups := map[string]routing.Lookup{}
	for _, id := range []string{processingDB, other} {
		m, err := f.p.mysql["us-ashburn-1"](leaseBudget(t), id)
		if err != nil {
			t.Fatal(err)
		}
		lookups[id] = routing.Lookup{State: routing.Found, Metadata: m}
	}
	old, err := f.p.observer.Observe(noticeledger.Record{}, n, true, lookups, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	body, _ := old.Encode()
	f.live.objects.seed(noticeName(n.Key().NoticeID), body)
	q := reviewedRecovery(t, old, noticeledger.ApproveRevision, n.Key().Digest)
	f.configure(t, func(v map[string]any) { edit(v); v["schemaVersion"], v["recovery"] = 7, q })
	current := f.p.current
	f.p.current = func(ctx context.Context, id string) (maintenanceCredential, error) {
		if id == "ocid1.vaultsecret.oc1.iad.video" {
			return nil, nil
		}
		return current(ctx, id)
	}
	s := f.run(t)
	if s.Recovery == nil || s.Recovery.Reason != "secret_unavailable" || s.Prepared != 0 || f.sent.Load() != 0 || len(f.saved(t, n.Key().NoticeID).Snapshot().Batches) != 0 {
		t.Fatal("partial preparation escaped a rejected approval", s)
	}
}
