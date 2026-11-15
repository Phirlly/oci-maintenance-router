package reconciliation

import (
	"context"
	"testing"
	"time"

	"oci-maintenance-router/internal/noticeledger"
	"oci-maintenance-router/internal/notices"
	"oci-maintenance-router/internal/ocistorage"
)

func reviewFixture(t *testing.T, f *processingFixture) *RecoveryReviewer {
	t.Helper()
	f.configure(t, func(v map[string]any) { v["schemaVersion"] = 7; delete(v, "recovery") })
	store, err := ocistorage.NewNoticeStore(f.live.objects, f.live.lease.manager.environmentID)
	if err != nil {
		t.Fatal(err)
	}
	r, err := NewRecoveryReviewer(f.live.config, []byte(preparationCatalog), candidateLocation(), f.source, store, processingMySQL(t))
	if err != nil {
		t.Fatal(err)
	}
	r.mysql = f.p.mysql
	r.wait = func(ctx context.Context, _ time.Duration) error { return ctx.Err() }
	return r
}

func seedReviewRevision(t *testing.T, f *processingFixture, ns ...notices.Notice) noticeledger.Record {
	t.Helper()
	var record noticeledger.Record
	for _, n := range ns {
		var err error
		record, err = f.p.observer.Observe(record, n, true, nil, time.Now().Add(-time.Minute))
		if err != nil {
			t.Fatal(err)
		}
	}
	body, err := record.Encode()
	if err != nil {
		t.Fatal(err)
	}
	f.live.objects.seed(noticeName(ns[0].Key().NoticeID), body)
	return record
}

func reviewRequest(view RecoveryReview, id string, kind noticeledger.ActionKind, target string) map[string]any {
	return map[string]any{"requestId": "reviewed-1", "noticeId": id, "kind": string(kind), "targetId": target,
		"routingGeneration": view.Routing.RoutingGeneration, "componentFingerprint": view.Component.Fingerprint, "reviewFingerprint": view.Fingerprint}
}
