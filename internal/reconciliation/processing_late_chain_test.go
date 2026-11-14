package reconciliation

import (
	"context"
	"oci-maintenance-router/internal/delivery"
	"oci-maintenance-router/internal/noticeledger"
	"oci-maintenance-router/internal/notices"
	"oci-maintenance-router/internal/ocistorage"
	"testing"
	"time"
)

func TestMaintenanceLateChainChangeRetainsKnownUnionAndStopsAllDispatch(t *testing.T) {
	for _, fault := range []string{"none", "own_write", "peer_read", "peer_write", "peer_clock", "peer_recovery_schema"} {
		a := processingNotice(t, 1, processingDB)
		bRecord := processingNotice(t, 2, processingDB).Snapshot()
		bRecord.ChainID = chain("joined")
		b := normalizeCandidate(t, bRecord)
		f := processingFixtureFor(t, a, b)
		get := f.source.get
		var recoveryPeer []byte
		f.source.get = func(ctx context.Context, id string) (notices.Notice, error) {
			n, e := get(ctx, id)
			if id == a.Key().NoticeID && f.fetched.Load() >= 6 {
				if fault == "peer_recovery_schema" {
					peer := f.saved(t, b.Key().NoticeID)
					migrated, err := peer.MigrateRecovery(time.Now())
					if err != nil {
						t.Fatal(err)
					}
					recoveryPeer, err = migrated.Encode()
					if err != nil {
						t.Fatal(err)
					}
					f.live.objects.seed(noticeName(b.Key().NoticeID), recoveryPeer)
				}
				v := n.Snapshot()
				v.ChainID = chain("joined")
				return normalizeCandidate(t, v), nil
			}
			return n, e
		}
		f.p.discovery.collector.announcements = f.source
		changed := false
		isChange := func(c discoveryCall) bool {
			if c.name != noticeName(a.Key().NoticeID) || c.method != "REPLACE" {
				return false
			}
			r, e := noticeledger.Decode(c.body, f.live.lease.manager.environmentID, a.Key().NoticeID)
			return e == nil && len(r.Snapshot().Revisions) == 2
		}
		f.live.objects.before = func(_ context.Context, c discoveryCall) error {
			if fault == "own_write" && isChange(c) {
				return &ocistorage.Error{Code: ocistorage.Conflict, Status: 412}
			}
			if changed && c.name == noticeName(b.Key().NoticeID) && ((fault == "peer_read" && c.method == "GET") || (fault == "peer_write" && c.method == "REPLACE")) {
				return &ocistorage.Error{Code: ocistorage.Service, Status: 503}
			}
			return nil
		}
		f.live.objects.after = func(_ context.Context, c discoveryCall) error {
			if isChange(c) {
				changed = true
			}
			if changed && fault == "peer_clock" && c.name == noticeName(b.Key().NoticeID) && c.method == "GET" {
				f.live.lease.manager.now = func() time.Time { return time.Unix(1, 0) }
			}
			return nil
		}
		r, err := f.p.Run(leaseBudget(t), f.live.lease)
		if err == nil || r.Snapshot().Completed || f.sent.Load() != 0 {
			t.Fatal("late chain change authorized another send", fault)
		}
		f.live.objects.before, f.live.objects.after = nil, nil
		own := f.saved(t, a.Key().NoticeID).Snapshot()
		if fault != "own_write" && (len(own.Revisions) != 2 || own.HoldReason != delivery.ReasonSourceChanged) {
			t.Fatal("confirmed changed observation lost")
		}
		if fault == "none" && f.saved(t, b.Key().NoticeID).Snapshot().HoldReason != delivery.ReasonSourceChanged {
			t.Fatal("known linked peer not held")
		}
		if fault == "peer_recovery_schema" {
			got := f.saved(t, b.Key().NoticeID).Snapshot()
			if len(recoveryPeer) == 0 || got.SchemaVersion != 3 || got.HoldReason != delivery.ReasonSourceChanged || len(got.Recovery.Actions) != 0 {
				t.Fatal("late-chain handling failed to hold recovery-schema peer")
			}
		}
	}
}
