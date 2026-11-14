package reconciliation

import (
	"bytes"
	"context"
	"oci-maintenance-router/internal/delivery"
	"oci-maintenance-router/internal/noticeledger"
	"oci-maintenance-router/internal/notices"
	"oci-maintenance-router/internal/pagerduty"
	"oci-maintenance-router/internal/routing"
	"testing"
	"time"
)

func TestRecoveryManualRetryUsesSavedIdentityOnce(t *testing.T) {
	for _, status := range []int{202, 429, 503, 0} {
		n := processingNotice(t, 1, processingDB)
		f := processingFixtureWithBaseline(t, []notices.Notice{n}, n)
		old := f.seedBatch(t, n, time.Now().Add(-25*time.Hour), delivery.RetryWait, true)
		b := old.Snapshot().Batches[0]
		q := reviewedRecovery(t, old, noticeledger.RetryBatch, b.Message.BatchID)
		f.configure(t, func(v map[string]any) { v["schemaVersion"], v["recovery"] = 7, q })
		send := f.p.senders["US"]
		f.p.senders["US"] = func(ctx context.Context, body, key []byte) (pagerduty.Response, error) {
			response, err := send(ctx, body, key)
			response.Status = status
			return response, err
		}
		s := f.run(t)
		got := f.saved(t, n.Key().NoticeID).Snapshot().Batches[0]
		a := recoveryActions(t, f, n.Key().NoticeID)[0]
		want := noticeledger.RecoveryReviewed
		if status == 202 {
			want = noticeledger.RecoveryAccepted
		}
		if a.Status != want || s.Recovery == nil || s.Recovery.Status != string(want) || f.sent.Load() != 1 || f.current.Load() != 0 || f.exact.Load() != 1 || got.AttemptCount != b.AttemptCount+1 || !got.FirstAttemptAt.Equal(b.FirstAttemptAt) || got.Message.PayloadDigest != b.Message.PayloadDigest || got.SecretVersion != b.SecretVersion || !bytes.Equal(got.Message.PayloadWithoutRoutingKey, b.Message.PayloadWithoutRoutingKey) {
			t.Fatal("manual outcome or identity mismatch", status, s, a.Status)
		}
		f.run(t)
		if f.sent.Load() != 1 || len(recoveryActions(t, f, n.Key().NoticeID)) != 1 {
			t.Fatal("retained manual request resent")
		}
	}
}

func TestRecoveryApprovalPreparesOnlyExplicitBaselineWork(t *testing.T) {
	n := processingNotice(t, 1, processingDB)
	f := processingFixtureWithBaseline(t, []notices.Notice{n}, n)
	metadata, err := f.p.mysql["us-ashburn-1"](leaseBudget(t), processingDB)
	if err != nil {
		t.Fatal(err)
	}
	lookups := map[string]routing.Lookup{processingDB: {State: routing.Found, Metadata: metadata}}
	old, err := f.p.observer.Observe(noticeledger.Record{}, n, true, lookups, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	body, _ := old.Encode()
	f.live.objects.seed(noticeName(n.Key().NoticeID), body)
	q := reviewedRecovery(t, old, noticeledger.ApproveRevision, n.Key().Digest)
	f.configure(t, func(v map[string]any) { v["schemaVersion"], v["recovery"] = 7, q })
	s := f.run(t)
	a := recoveryActions(t, f, n.Key().NoticeID)[0]
	if a.Status != noticeledger.RecoveryApproved || len(a.BatchIDs) != 1 || s.Prepared != 1 || s.Accepted != 1 || f.sent.Load() != 1 {
		t.Fatal("explicit baseline approval did not prepare and deliver fixed work", s, a)
	}
	f.run(t)
	if f.sent.Load() != 1 {
		t.Fatal("approved baseline request replayed")
	}
}
