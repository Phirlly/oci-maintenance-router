package reconciliation

import (
	"bytes"
	"context"
	"testing"
	"time"

	"oci-maintenance-router/internal/notices"
	"oci-maintenance-router/internal/routing"
)

func TestLegacyProcessingRejectsAnnouncementMigrationAfterDiscovery(t *testing.T) {
	n := processingNotice(t, 1, processingDB)
	f := processingFixtureFor(t, n)
	now := f.p.discovery.now
	reads, lookups := 0, 0
	var migrated []byte
	f.p.mysql["us-ashburn-1"] = func(context.Context, string) (routing.Metadata, error) { lookups++; return routing.Metadata{}, nil }
	f.p.discovery.now = func() time.Time {
		at := now()
		reads++
		if reads == 4 {
			next := announcementRecordFor(t, f.saved(t, n.Key().NoticeID))
			migrated, _ = next.Encode()
			f.live.objects.seed(noticeName(n.Key().NoticeID), migrated)
		}
		return at
	}
	got, err := f.p.Run(leaseBudget(t), f.live.lease)
	after, _ := f.saved(t, n.Key().NoticeID).Encode()
	if err == nil || got.Snapshot().Completed || len(migrated) == 0 || !bytes.Equal(migrated, after) || lookups != 0 || f.current.Load() != 0 || f.exact.Load() != 0 || f.sent.Load() != 0 {
		t.Fatal("old controls adopted newer record", err, got.Snapshot())
	}
}

func TestLegacyProcessingRejectsAnnouncementPeerBeforeHold(t *testing.T) {
	a := processingNotice(t, 1, processingDB)
	bSource := processingNotice(t, 2, processingDB).Snapshot()
	bSource.ChainID = chain("joined")
	b := normalizeCandidate(t, bSource)
	f := processingFixtureFor(t, a, b)
	get := f.source.get
	var migrated []byte
	lookups := 0
	f.p.mysql["us-ashburn-1"] = func(context.Context, string) (routing.Metadata, error) { lookups++; return routing.Metadata{}, nil }
	f.source.get = func(ctx context.Context, id string) (notices.Notice, error) {
		n, err := get(ctx, id)
		if id == a.Key().NoticeID && f.fetched.Load() >= 5 {
			old := f.saved(t, b.Key().NoticeID)
			three, failure := old.MigrateRecovery(time.Now())
			if failure != nil {
				t.Fatal(failure)
			}
			four, failure := three.MigrateAnnouncements(time.Now())
			if failure != nil {
				t.Fatal(failure)
			}
			migrated, _ = four.Encode()
			f.live.objects.seed(noticeName(b.Key().NoticeID), migrated)
			changed := n.Snapshot()
			changed.ChainID = chain("joined")
			return normalizeCandidate(t, changed), nil
		}
		return n, err
	}
	f.p.discovery.collector.announcements = f.source
	got, err := f.p.Run(leaseBudget(t), f.live.lease)
	after, _ := f.saved(t, b.Key().NoticeID).Encode()
	if err == nil || got.Snapshot().Completed || len(migrated) == 0 || !bytes.Equal(migrated, after) || lookups != 0 || f.current.Load() != 0 || f.sent.Load() != 0 {
		t.Fatal("older processing changed new peer", err)
	}
}
