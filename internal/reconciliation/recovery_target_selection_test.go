package reconciliation

import (
	"context"
	"oci-maintenance-router/internal/delivery"
	"oci-maintenance-router/internal/noticeledger"
	"oci-maintenance-router/internal/notices"
	"oci-maintenance-router/internal/routing"
	"strings"
	"testing"
	"time"
)

func TestRecoveryRejectsIneligibleTargetsWithoutManualDispatch(t *testing.T) {
	for _, fault := range []string{"active", "prepared", "missing-revision", "no-owner", "unsupported"} {
		n := processingNotice(t, 1, processingDB)
		if fault == "unsupported" {
			v := n.Snapshot()
			v.AnnouncementType = "OTHER"
			n = normalizeCandidate(t, v)
		}
		f := processingFixtureWithBaseline(t, []notices.Notice{n}, n)
		if fault == "no-owner" {
			lookup := f.p.mysql["us-ashburn-1"]
			f.p.mysql["us-ashburn-1"] = func(ctx context.Context, id string) (routing.Metadata, error) {
				m, e := lookup(ctx, id)
				m.FreeformTags = nil
				return m, e
			}
		}
		old := seedRecoveryRevision(t, f, n)
		kind, target, want := noticeledger.ApproveRevision, n.Key().Digest, "no_eligible_work"
		if fault == "active" || fault == "prepared" {
			state := delivery.Prepared
			if fault == "active" {
				state, want = delivery.Sending, "active_attempt"
			}
			old = f.seedBatch(t, n, time.Now().Add(-time.Second), state, false)
			kind, target = noticeledger.RetryBatch, old.Snapshot().Batches[0].Message.BatchID
		}
		q := reviewedRecovery(t, old, kind, target)
		if fault == "missing-revision" {
			q["targetId"], want = strings.Repeat("f", 64), "target_missing"
		}
		f.configure(t, func(v map[string]any) { v["schemaVersion"], v["recovery"] = 7, q })
		s := f.run(t)
		if s.Recovery == nil || s.Recovery.Status != string(noticeledger.RecoveryRejected) || s.Recovery.Reason != want || f.sent.Load() != 0 || f.exact.Load() != 0 || f.current.Load() != 0 {
			t.Fatal("ineligible target obtained recovery authority", fault, s)
		}
	}
}

func TestRecoveryDoesNotSelectHistoricalRevisionWhenCurrentSourceChanged(t *testing.T) {
	previous := processingNotice(t, 1, processingDB)
	v := previous.Snapshot()
	v.Reference = chain("new revision")
	current := normalizeCandidate(t, v)
	f := processingFixtureFor(t, current)
	old := f.seedBatch(t, previous, time.Now().Add(-time.Minute), delivery.Prepared, true)
	m, err := f.p.mysql["us-ashburn-1"](leaseBudget(t), processingDB)
	if err != nil {
		t.Fatal(err)
	}
	retained, err := f.p.observer.Observe(old, current, true, map[string]routing.Lookup{processingDB: {State: routing.Found, Metadata: m}}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	body, _ := retained.Encode()
	f.live.objects.seed(noticeName(current.Key().NoticeID), body)
	q := reviewedRecovery(t, retained, noticeledger.RetryBatch, old.Snapshot().Batches[0].Message.BatchID)
	f.configure(t, func(v map[string]any) { v["schemaVersion"], v["recovery"] = 7, q })
	s := f.run(t)
	if s.Recovery == nil || s.Recovery.Reason != "source_changed" || f.sent.Load() != 0 || f.exact.Load() != 0 {
		t.Fatal("historical revision selected as current", s)
	}
}
