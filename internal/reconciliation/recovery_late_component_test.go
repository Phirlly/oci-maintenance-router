package reconciliation

import (
	"context"
	"encoding/json"
	"oci-maintenance-router/internal/delivery"
	"oci-maintenance-router/internal/noticeledger"
	"oci-maintenance-router/internal/notices"
	"strings"
	"testing"
	"time"
)

func linkedRecoveryFixture(t *testing.T) (*processingFixture, notices.Notice, notices.Notice) {
	t.Helper()
	peerSource := processingNotice(t, 1, processingDB).Snapshot()
	peerSource.ChainID = chain("original")
	peer := normalizeCandidate(t, peerSource)
	targetSource := processingNotice(t, 2, processingDB).Snapshot()
	targetSource.ChainID = chain("original")
	target := normalizeCandidate(t, targetSource)
	f := processingFixtureFor(t, peer, target)
	component := componentFingerprint(f.live.lease.manager.environmentID, []CandidateRevision{
		{Revision: peer.Key(), ChainID: "original"}, {Revision: target.Key(), ChainID: "original"},
	})
	old := f.seedBatch(t, target, time.Now().Add(-time.Minute), delivery.Prepared, true)
	seedApprovedRecoveryForComponent(t, f, peer, true, component)
	projected, err := old.MigrateRecovery(time.Now())
	if err != nil {
		t.Fatal(err)
	}
	q := reviewedRecovery(t, projected, noticeledger.RetryBatch, old.Snapshot().Batches[0].Message.BatchID)
	q["componentFingerprint"] = component
	q["reviewFingerprint"], err = projected.ReviewFingerprint(noticeledger.RetryBatch, q["targetId"].(string), q["routingGeneration"].(string), component)
	if err != nil {
		t.Fatal(err)
	}
	f.configure(t, func(v map[string]any) { v["schemaVersion"], v["recovery"] = 7, q })
	return f, peer, target
}

func TestRecoveryEarlyResultCannotContinueAfterObservedMembershipChange(t *testing.T) {
	for _, retained := range []bool{false, true} {
		name := "missing-target"
		if retained {
			name = "retained-intent"
		}
		t.Run(name, func(t *testing.T) {
			f, _, target := linkedRecoveryFixture(t)
			if !retained {
				var input map[string]any
				if err := json.Unmarshal(f.live.config, &input); err != nil {
					t.Fatal(err)
				}
				q := input["recovery"].(map[string]any)
				q["targetId"] = strings.Repeat("f", 64)
				f.configure(t, func(v map[string]any) { v["schemaVersion"], v["recovery"] = 7, q })
			}
			now, calls := f.p.discovery.now, 0
			f.p.discovery.now = func() time.Time {
				at := now()
				calls++
				if calls == 6 {
					v := target.Snapshot()
					v.ChainID = chain("new-group")
					r, err := f.p.observer.Observe(f.saved(t, target.Key().NoticeID), normalizeCandidate(t, v), true, nil, at)
					if err != nil {
						t.Fatal(err)
					}
					if retained {
						r, _, err = r.RejectRecovery((&processingRun{owner: f.p}).recoveryRequest(), "no_eligible_work", at)
						if err != nil {
							t.Fatal(err)
						}
					}
					body, _ := r.Encode()
					f.live.objects.seed(noticeName(target.Key().NoticeID), body)
				}
				return at
			}
			r, err := f.p.Run(leaseBudget(t), f.live.lease)
			if calls < 6 || err == nil || r.Snapshot().Completed || f.sent.Load() != 0 {
				t.Fatal("early recovery result retained stale dispatch authority", retained, err, f.sent.Load())
			}
			if retained && (r.Snapshot().Recovery == nil || r.Snapshot().Recovery.Reason != "no_eligible_work") {
				t.Fatal("membership failure hid concurrently retained intent", err)
			}
		})
	}
}

func TestRecoveryLateSourceChangeInvalidatesWholeUnionAndStopsOldWorklist(t *testing.T) {
	f, peer, target := linkedRecoveryFixture(t)
	get, targetReads := f.source.get, 0
	f.source.get = func(ctx context.Context, id string) (notices.Notice, error) {
		n, err := get(ctx, id)
		if id == target.Key().NoticeID {
			targetReads++
			if targetReads >= 3 {
				v := n.Snapshot()
				v.ChainID = chain("new-group")
				return normalizeCandidate(t, v), err
			}
		}
		return n, err
	}
	f.p.discovery.collector.announcements = f.source
	r, err := f.p.Run(leaseBudget(t), f.live.lease)
	a := recoveryActions(t, f, peer.Key().NoticeID)[0]
	if err == nil || r.Snapshot().Completed || f.sent.Load() != 0 || a.InvalidatedAt == nil || a.InvalidationReason != "source_changed" || r.Snapshot().Recovery == nil || r.Snapshot().Recovery.Reason != "source_changed" {
		t.Fatal("late changed component retained send authority", err, f.sent.Load(), a.InvalidatedAt, r.Snapshot().Recovery)
	}
}

func TestRecoveryConcurrentSavedMembershipChangeStopsOldWorklist(t *testing.T) {
	f, _, target := linkedRecoveryFixture(t)
	now, calls := f.p.discovery.now, 0
	f.p.discovery.now = func() time.Time {
		at := now()
		calls++
		// Two observations and completion in each complete discovery pass.
		if calls == 6 {
			v := target.Snapshot()
			v.ChainID = chain("new-group")
			r, err := f.p.observer.Observe(f.saved(t, target.Key().NoticeID), normalizeCandidate(t, v), true, nil, at)
			if err != nil {
				t.Fatal(err)
			}
			body, _ := r.Encode()
			f.live.objects.seed(noticeName(target.Key().NoticeID), body)
		}
		return at
	}
	r, err := f.p.Run(leaseBudget(t), f.live.lease)
	if calls < 6 || err == nil || r.Snapshot().Completed || f.sent.Load() != 0 {
		t.Fatal("observed saved membership change retained stale send authority", calls, err, f.sent.Load())
	}
}
