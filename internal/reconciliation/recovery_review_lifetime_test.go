package reconciliation

import (
	"context"
	"errors"
	"testing"
	"time"

	"oci-maintenance-router/internal/noticeledger"
	"oci-maintenance-router/internal/notices"
	"oci-maintenance-router/internal/ociannouncements"
	"oci-maintenance-router/internal/routing"
)

type reviewSavedSource struct {
	SavedNoticeSource
	load func(context.Context, string) (noticeledger.Record, error)
}

func TestRecoveryReviewStopsIfPacingFailsAfterCollection(t *testing.T) {
	for _, stage := range []string{"reload", "source", "lookup"} {
		t.Run(stage, func(t *testing.T) {
			n := processingNotice(t, 1, processingDB)
			f := processingFixtureFor(t, n)
			seedReviewRevision(t, f, n)
			r := reviewFixture(t, f)
			old, loads := r.collector.saved, 0
			r.collector.saved = reviewSavedSource{SavedNoticeSource: old, load: func(ctx context.Context, name string) (noticeledger.Record, error) {
				loads++
				return old.LoadRecord(ctx, name)
			}}
			r.wait = func(ctx context.Context, _ time.Duration) error {
				if stage == "reload" && loads >= 1 || loads >= 2 && (stage == "source" || f.fetched.Load() >= 2) {
					return errors.New("private pacing failure")
				}
				return ctx.Err()
			}
			lookups := 0
			r.mysql["us-ashburn-1"] = func(context.Context, string) (routing.Metadata, error) {
				lookups++
				t.Error("lookup after failed pacing")
				return routing.Metadata{}, nil
			}
			view, err := r.Review(leaseBudget(t), n.Key().NoticeID, noticeledger.ApproveRevision, n.Key().Digest)
			wantReads := int32(1)
			if stage == "lookup" {
				wantReads = 2
			}
			if err == nil || err.Error() != "recovery review: wait_failed" || view.Saved.Snapshot().NoticeID != "" || view.Fingerprint != "" || lookups != 0 || f.fetched.Load() != wantReads {
				t.Fatal("failed pacing continued or returned partial review", stage, err)
			}
		})
	}
}

func TestRecoveryReviewRetainsShortOriginalDeadlineThroughLookup(t *testing.T) {
	n := processingNotice(t, 1, processingDB)
	f := processingFixtureFor(t, n)
	seedReviewRevision(t, f, n)
	r := reviewFixture(t, f)
	// Collection is deterministic and immediate; the real parent deadline expires
	// while its final metadata read waits, proving no stage renews that budget.
	ctx, cancel := context.WithTimeout(leaseBudget(t), time.Second)
	defer cancel()
	end, _ := ctx.Deadline()
	called := false
	r.mysql["us-ashburn-1"] = func(work context.Context, _ string) (routing.Metadata, error) {
		called = true
		got, ok := work.Deadline()
		if !ok || !got.Equal(end) {
			t.Error("original deadline changed")
		}
		<-work.Done()
		return routing.Metadata{}, work.Err()
	}
	view, err := r.Review(ctx, n.Key().NoticeID, noticeledger.ApproveRevision, n.Key().Digest)
	if !called || err == nil || err.Error() != "recovery review: deadline_exceeded" || view.Saved.Snapshot().NoticeID != "" || view.Fingerprint != "" {
		t.Fatal("expired read returned partial evidence", err)
	}
}

func (s reviewSavedSource) LoadRecord(ctx context.Context, name string) (noticeledger.Record, error) {
	return s.load(ctx, name)
}

func TestRecoveryReviewCancellationDiscardsBlockedEvidence(t *testing.T) {
	n := processingNotice(t, 1, processingDB)
	f := processingFixtureFor(t, n)
	seedReviewRevision(t, f, n)
	r := reviewFixture(t, f)
	r.collector.announcements = announcementDetails{
		page: func(context.Context, string) (ociannouncements.Page, error) { return ociannouncements.Page{}, nil },
		get: func(context.Context, string) (notices.Notice, error) {
			return notices.Notice{}, &ociannouncements.Error{Code: ociannouncements.Service, Status: 404}
		},
	}
	ctx, cancel := context.WithCancel(leaseBudget(t))
	defer cancel()
	old, calls := r.collector.saved, 0
	r.collector.saved = reviewSavedSource{SavedNoticeSource: old, load: func(ctx context.Context, name string) (noticeledger.Record, error) {
		v, err := old.LoadRecord(ctx, name)
		calls++
		if calls == 2 {
			cancel()
		}
		return v, err
	}}
	view, err := r.Review(ctx, n.Key().NoticeID, noticeledger.ApproveRevision, n.Key().Digest)
	if err == nil || view.Saved.Snapshot().NoticeID != "" || view.Blocker != "" || view.Fingerprint != "" {
		t.Fatal("canceled review returned partial evidence", view.Blocker, err)
	}
}
