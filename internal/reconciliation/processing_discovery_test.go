package reconciliation

import (
	"context"
	"fmt"
	"oci-maintenance-router/internal/delivery"
	"oci-maintenance-router/internal/noticeledger"
	"oci-maintenance-router/internal/notices"
	"oci-maintenance-router/internal/ociannouncements"
	"oci-maintenance-router/internal/ocistorage"
	"testing"
)

func TestMaintenanceAvailableChangedSourceIsNotReportedUnavailable(t *testing.T) {
	n := processingNotice(t, 1, processingDB)
	var changed notices.Notice
	for i := 0; i < 100; i++ {
		v := n.Snapshot()
		v.Reference = chain(fmt.Sprint("revision-", i))
		changed = normalizeCandidate(t, v)
		if changed.Key().Digest < n.Key().Digest {
			break
		}
	}
	if changed.Key().Digest >= n.Key().Digest {
		t.Fatal("fixture needs historical digest ordered last")
	}
	f := processingFixtureFor(t, n)
	get := f.source.get
	f.source.get = func(ctx context.Context, id string) (notices.Notice, error) {
		if f.listed.Load() >= 2 {
			return changed, nil
		}
		return get(ctx, id)
	}
	f.p.discovery.collector.announcements = f.source
	s := f.run(t)
	if len(s.Issues) != 1 || s.Issues[0].Code != "source_changed" || f.saved(t, n.Key().NoticeID).Snapshot().SourceOutcome.State != noticeledger.SourceAvailable {
		t.Fatalf("available changed source misreported: %+v", s.Issues)
	}
}

func TestMaintenanceFinalOnlyArrivalPersistsThenWaitsForNextPass(t *testing.T) {
	a, b := processingNotice(t, 1, processingDB), processingNotice(t, 2, processingDB)
	f := processingFixtureFor(t, a, b)
	page := f.source.page
	f.source.page = func(ctx context.Context, cursor string) (ociannouncements.Page, error) {
		p, e := page(ctx, cursor)
		if f.listed.Load() == 1 {
			p.IDs = p.IDs[:1]
		}
		return p, e
	}
	f.p.discovery.collector.announcements = f.source
	s := f.run(t)
	if s.Accepted != 1 || s.Deferred != 1 || len(f.saved(t, b.Key().NoticeID).Snapshot().Batches) != 0 {
		t.Fatal("new final arrival sent or lost")
	}
	again := f.run(t)
	if again.Accepted != 1 || f.sent.Load() != 2 {
		t.Fatal("deferred arrival did not process on next complete pass")
	}
}

func TestMaintenanceFinalChangedUnionHoldsAllMembers(t *testing.T) {
	a := processingNotice(t, 1, processingDB)
	bRecord := processingNotice(t, 2, processingDB).Snapshot()
	bRecord.ChainID = chain("joined")
	b := normalizeCandidate(t, bRecord)
	f := processingFixtureFor(t, a, b)
	page, get := f.source.page, f.source.get
	f.source.page = func(ctx context.Context, cursor string) (ociannouncements.Page, error) {
		p, e := page(ctx, cursor)
		if f.listed.Load() == 1 {
			p.IDs = p.IDs[:1]
		}
		return p, e
	}
	f.source.get = func(ctx context.Context, id string) (notices.Notice, error) {
		n, e := get(ctx, id)
		if f.listed.Load() >= 2 && id == a.Key().NoticeID {
			v := n.Snapshot()
			v.ChainID = chain("joined")
			n = normalizeCandidate(t, v)
		}
		return n, e
	}
	f.p.discovery.collector.announcements = f.source
	s := f.run(t)
	if !s.Completed || f.sent.Load() != 0 || len(s.Issues) != 2 {
		t.Fatal("changed union not withheld")
	}
	for _, n := range []notices.Notice{a, b} {
		if f.saved(t, n.Key().NoticeID).Snapshot().HoldReason != delivery.ReasonSourceChanged {
			t.Fatal("union hold absent")
		}
	}
}

func TestMaintenanceFinalUnavailableSourceRemainsVisible(t *testing.T) {
	n := processingNotice(t, 1, processingDB)
	f := processingFixtureFor(t, n)
	page, get := f.source.page, f.source.get
	f.source.page = func(ctx context.Context, cursor string) (ociannouncements.Page, error) {
		p, e := page(ctx, cursor)
		if f.listed.Load() >= 2 {
			p.IDs = nil
		}
		return p, e
	}
	f.source.get = func(ctx context.Context, id string) (notices.Notice, error) {
		if f.listed.Load() >= 2 {
			return notices.Notice{}, &ociannouncements.Error{Code: ociannouncements.Service, Status: 404}
		}
		return get(ctx, id)
	}
	f.p.discovery.collector.announcements = f.source
	s := f.run(t)
	record := f.saved(t, n.Key().NoticeID).Snapshot()
	if !s.Completed || len(s.Issues) != 1 || f.sent.Load() != 0 || record.HoldReason != delivery.ReasonSourceUnavailable || record.SourceOutcome.HTTPStatus != 404 {
		t.Fatal("saved-only failure lost or dispatched")
	}
}

func TestMaintenanceCrossPassLostHistoryAndPartialPersistenceBlockSending(t *testing.T) {
	for _, lost := range []bool{false, true} {
		n := processingNotice(t, 1, processingDB)
		f := processingFixtureFor(t, n)
		if lost {
			changed := n.Snapshot()
			changed.Reference = chain("replacement")
			replacement := normalizeCandidate(t, changed)
			page, get := f.source.page, f.source.get
			f.source.page = func(ctx context.Context, cursor string) (ociannouncements.Page, error) {
				p, e := page(ctx, cursor)
				if f.listed.Load() == 2 {
					r, err := f.p.observer.Observe(noticeledger.Record{}, replacement, true, nil, f.live.lease.manager.now())
					if err != nil {
						t.Fatal(err)
					}
					body, _ := r.Encode()
					f.live.objects.seed(noticeName(n.Key().NoticeID), body)
				}
				return p, e
			}
			f.source.get = func(ctx context.Context, id string) (notices.Notice, error) {
				if f.listed.Load() >= 2 {
					return replacement, nil
				}
				return get(ctx, id)
			}
			f.p.discovery.collector.announcements = f.source
		} else {
			f.live.objects.before = func(_ context.Context, c discoveryCall) error {
				if f.listed.Load() == 2 && c.name == noticeName(n.Key().NoticeID) && c.method == "REPLACE" {
					return &ocistorage.Error{Code: ocistorage.Service, Status: 503}
				}
				return nil
			}
		}
		r, err := f.p.Run(leaseBudget(t), f.live.lease)
		if err == nil || r.Snapshot().Completed || f.sent.Load() != 0 {
			t.Fatal("incomplete/lost final history authorized send")
		}
		if lost {
			e, ok := err.(*ProcessingError)
			if !ok || e.Code != "membership_lost" {
				t.Fatal("wrong lost-history result", err)
			}
		}
	}
}
