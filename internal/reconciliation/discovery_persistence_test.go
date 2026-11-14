package reconciliation

import (
	"context"
	"testing"

	"oci-maintenance-router/internal/delivery"
	"oci-maintenance-router/internal/noticeledger"
	"oci-maintenance-router/internal/notices"
	"oci-maintenance-router/internal/ociannouncements"
	"oci-maintenance-router/internal/ocistorage"
)

func TestDiscoveryMigratesSavedOnlyBeforeUnavailableOutcome(t *testing.T) {
	n := candidateNotice(t, 1, nil, true)
	old := discoveryLegacy(t, candidateRecord(t, n))
	if old.Snapshot().SchemaVersion != 1 {
		t.Fatal("fixture must be legacy")
	}
	d, s, lease := discoveryFixture(t, nil, []noticeledger.Record{old})
	got, err := d.Run(leaseBudget(t), lease)
	if err != nil {
		t.Fatal(err)
	}
	writes := s.noticeWrites()
	if len(writes) != 2 {
		t.Fatal("migration and outcome require distinct saves", len(writes))
	}
	for i, call := range writes {
		r, err := noticeledger.Decode(call.body, got.EnvironmentID(), n.Key().NoticeID)
		if err != nil {
			t.Fatal(err)
		}
		want := noticeledger.SourceUnknown
		if i == 1 {
			want = noticeledger.SourceUnavailable
		}
		if call.method != "REPLACE" || r.Snapshot().SourceOutcome.State != want {
			t.Fatal("wrong transition order")
		}
	}
	current := s.record(t, got.EnvironmentID(), n.Key().NoticeID).Snapshot()
	if current.SourceOutcome.HTTPStatus != 404 || current.HoldReason != delivery.ReasonSourceUnavailable || len(got.Availability().Unavailable()) != 1 {
		t.Fatal("missing durable unavailable evidence")
	}
}

func TestDiscoveryDoesNotWriteBeforeCompleteCollection(t *testing.T) {
	n := candidateNotice(t, 1, nil, true)
	d, s, lease := discoveryFixture(t, []notices.Notice{n, candidateNotice(t, 2, nil, true)}, nil)
	a := d.collector.announcements.(announcementDetails)
	original := a.get
	calls := 0
	a.get = func(ctx context.Context, id string) (notices.Notice, error) {
		calls++
		if calls == 2 {
			return notices.Notice{}, &ociannouncements.Error{Code: ociannouncements.Service, Status: 503}
		}
		return original(ctx, id)
	}
	d.collector.announcements = a
	got, err := d.Run(leaseBudget(t), lease)
	discoveryFailure(t, got, err, "collection_failed")
	if len(s.noticeWrites()) != 0 {
		t.Fatal("incomplete discovery wrote an observation")
	}
}

func TestDiscoveryPartialDurabilityReturnsNoCompletion(t *testing.T) {
	a, b := candidateNotice(t, 1, nil, true), candidateNotice(t, 2, nil, true)
	d, s, lease := discoveryFixture(t, []notices.Notice{a, b}, nil)
	s.before = func(_ context.Context, call discoveryCall) error {
		if call.method == "CREATE" && call.name == noticeName(b.Key().NoticeID) {
			return &ocistorage.Error{Code: ocistorage.Service, Status: 403}
		}
		return nil
	}
	got, err := d.Run(leaseBudget(t), lease)
	discoveryFailure(t, got, err, "write_failed")
	if len(s.record(t, lease.manager.environmentID, a.Key().NoticeID).Snapshot().Revisions) != 1 {
		t.Fatal("confirmed partial state was lost")
	}
	if _, ok := s.objects[noticeName(b.Key().NoticeID)]; ok {
		t.Fatal("failed write reported as durable")
	}
}
