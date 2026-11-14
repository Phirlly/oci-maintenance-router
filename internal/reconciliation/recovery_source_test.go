package reconciliation

import (
	"context"
	"oci-maintenance-router/internal/delivery"
	"oci-maintenance-router/internal/noticeledger"
	"oci-maintenance-router/internal/notices"
	"oci-maintenance-router/internal/ociannouncements"
	"testing"
	"time"
)

func TestRecoveryUnavailableSavedSourceRejectsWithoutReadingSecret(t *testing.T) {
	n := processingNotice(t, 1, processingDB)
	f := processingFixtureFor(t, n)
	old := f.seedBatch(t, n, time.Now().Add(-time.Minute), delivery.Prepared, true)
	q := reviewedRecovery(t, old, noticeledger.RetryBatch, old.Snapshot().Batches[0].Message.BatchID)
	f.configure(t, func(v map[string]any) { v["schemaVersion"], v["recovery"] = 7, q })
	f.p.discovery.collector.announcements = announcementDetails{
		page: func(context.Context, string) (ociannouncements.Page, error) { return ociannouncements.Page{}, nil },
		get: func(context.Context, string) (notices.Notice, error) {
			return notices.Notice{}, &ociannouncements.Error{Code: ociannouncements.Service, Status: 404}
		},
	}
	s := f.run(t)
	if s.Recovery == nil || s.Recovery.Reason != "source_unavailable" || f.exact.Load() != 0 || f.sent.Load() != 0 || len(f.saved(t, n.Key().NoticeID).Snapshot().Batches) != 1 {
		t.Fatal("saved-source failure was discarded or sent", s)
	}
}

func TestRecoveryFreshSourceFailureAfterCompletePassCannotSend(t *testing.T) {
	for _, malformed := range []bool{false, true} {
		n := processingNotice(t, 1, processingDB)
		f := processingFixtureFor(t, n)
		old := f.seedBatch(t, n, time.Now().Add(-time.Minute), delivery.Prepared, true)
		q := reviewedRecovery(t, old, noticeledger.RetryBatch, old.Snapshot().Batches[0].Message.BatchID)
		f.configure(t, func(v map[string]any) { v["schemaVersion"], v["recovery"] = 7, q })
		get := f.source.get
		f.source.get = func(ctx context.Context, id string) (notices.Notice, error) {
			value, err := get(ctx, id)
			if f.fetched.Load() >= 3 {
				code := ociannouncements.Service
				if malformed {
					code = ociannouncements.InvalidResponse
				}
				return notices.Notice{}, &ociannouncements.Error{Code: code, Status: 503}
			}
			return value, err
		}
		f.p.discovery.collector.announcements = f.source
		r, err := f.p.Run(leaseBudget(t), f.live.lease)
		if f.sent.Load() != 0 || f.exact.Load() != 0 {
			t.Fatal("fresh failed source dispatched")
		}
		if malformed {
			if err == nil || r.Snapshot().Recovery != nil {
				t.Fatal("malformed source fabricated controlled rejection", err)
			}
		} else if err != nil || r.Snapshot().Recovery == nil || r.Snapshot().Recovery.Reason != "source_unavailable" {
			t.Fatal("known source failure did not retain rejection", err)
		}
	}
}
