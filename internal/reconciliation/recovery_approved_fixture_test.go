package reconciliation

import (
	"oci-maintenance-router/internal/delivery"
	"oci-maintenance-router/internal/noticeledger"
	"oci-maintenance-router/internal/notices"
	"oci-maintenance-router/internal/routing"
	"testing"
	"time"
)

func seedApprovedRecovery(t *testing.T, f *processingFixture, n notices.Notice, held bool) noticeledger.Record {
	t.Helper()
	return seedApprovedRecoveryForComponent(t, f, n, held, "")
}

func seedApprovedRecoveryForComponent(t *testing.T, f *processingFixture, n notices.Notice, held bool, component string) noticeledger.Record {
	t.Helper()
	at := time.Now().Add(-time.Minute)
	lookups := map[string]routing.Lookup{}
	ids := []string{}
	_, targets := f.p.policy.Targets(n, true)
	for _, target := range targets {
		metadata, err := f.p.mysql[target.Region](leaseBudget(t), target.ResourceID)
		if err != nil {
			t.Fatal(err)
		}
		lookups[target.ResourceID] = routing.Lookup{State: routing.Found, Metadata: metadata}
		ids = append(ids, target.ResourceID)
	}
	r, err := f.p.observer.Observe(noticeledger.Record{}, n, true, lookups, at)
	if err == nil && held {
		r, err = r.Hold(delivery.ReasonOperator, at)
	}
	if err == nil {
		r, err = r.MigrateRecovery(at)
	}
	if err != nil {
		t.Fatal(err)
	}
	values := reviewedRecovery(t, r, noticeledger.ApproveRevision, n.Key().Digest)
	q := noticeledger.RecoveryRequest{RequestID: values["requestId"].(string), Kind: noticeledger.ApproveRevision,
		TargetID: n.Key().Digest, RoutingGeneration: values["routingGeneration"].(string),
		ComponentFingerprint: values["componentFingerprint"].(string), ReviewFingerprint: values["reviewFingerprint"].(string)}
	if component != "" {
		q.ComponentFingerprint = component
		q.ReviewFingerprint, err = r.ReviewFingerprint(q.Kind, q.TargetID, q.RoutingGeneration, component)
		if err != nil {
			t.Fatal(err)
		}
	}
	drafts, err := f.p.builder.Build(n, true, lookups, ids)
	if err != nil || len(drafts.Issues) != 0 {
		t.Fatal(err, drafts.Issues)
	}
	prepared := []delivery.Preparation{}
	for _, draft := range drafts.Drafts {
		prepared = append(prepared, delivery.Preparation{Draft: draft, SecretVersion: 7})
	}
	r, a, err := f.p.observer.ApproveRecovery(r, q, n, true, lookups, at, prepared...)
	if err != nil || a.Status != noticeledger.RecoveryApproved {
		t.Fatal("fixture approval failed", err)
	}
	body, err := r.Encode()
	if err != nil {
		t.Fatal(err)
	}
	f.live.objects.seed(noticeName(n.Key().NoticeID), body)
	return r
}
