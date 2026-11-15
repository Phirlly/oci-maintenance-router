package reconciliation

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"oci-maintenance-router/internal/noticeledger"
	"oci-maintenance-router/internal/notices"
	"oci-maintenance-router/internal/ociannouncements"
)

func TestRecoveryReviewDiscardsEvidenceOnReadOrMembershipDrift(t *testing.T) {
	for _, kind := range []string{"list", "reload", "membership", "fresh-source", "fresh-error", "wait", "missing-target", "missing-notice"} {
		t.Run(kind, func(t *testing.T) {
			n := processingNotice(t, 1, processingDB)
			f := processingFixtureFor(t, n)
			seedReviewRevision(t, f, n)
			r := reviewFixture(t, f)
			id, target := n.Key().NoticeID, n.Key().Digest
			old := r.collector.saved
			calls := 0
			r.collector.saved = reviewSavedSource{SavedNoticeSource: old, load: func(ctx context.Context, name string) (noticeledger.Record, error) {
				v, err := old.LoadRecord(ctx, name)
				calls++
				if calls == 2 {
					if kind == "reload" {
						return noticeledger.Record{}, errors.New("private")
					}
					if kind == "membership" {
						raw := n.Snapshot()
						raw.Summary = chain("added saved revision")
						return f.p.observer.Observe(v, normalizeCandidate(t, raw), true, nil, time.Now())
					}
				}
				return v, err
			}}
			source := f.source
			reads := 0
			r.collector.announcements = announcementDetails{page: func(ctx context.Context, token string) (ociannouncements.Page, error) {
				if kind == "list" {
					return ociannouncements.Page{}, errors.New("private")
				}
				return source.page(ctx, token)
			}, get: func(ctx context.Context, id string) (notices.Notice, error) {
				reads++
				if reads == 2 {
					if kind == "fresh-error" {
						return notices.Notice{}, errors.New("private")
					}
					if kind == "fresh-source" {
						v := n.Snapshot()
						v.Summary = chain("changed")
						return normalizeCandidate(t, v), nil
					}
				}
				return source.get(ctx, id)
			}}
			if kind == "wait" {
				r.wait = func(context.Context, time.Duration) error { return errors.New("private") }
			}
			if kind == "missing-target" {
				target = strings.Repeat("f", 64)
			}
			if kind == "missing-notice" {
				id = noticeID(99)
			}
			view, err := r.Review(leaseBudget(t), id, noticeledger.ApproveRevision, target)
			if err == nil || strings.Contains(err.Error(), "private") || view.Saved.Snapshot().NoticeID != "" || view.Fingerprint != "" {
				t.Fatal("failed or changed review returned evidence", kind, err, view.Blocker)
			}
		})
	}
}

func TestRecoveryReviewSeparatesUnavailableAndReplacedSource(t *testing.T) {
	for _, unavailable := range []bool{true, false} {
		n := processingNotice(t, 1, processingDB)
		f := processingFixtureFor(t, n)
		seedReviewRevision(t, f, n)
		r := reviewFixture(t, f)
		if unavailable {
			r.collector.announcements = announcementDetails{page: func(context.Context, string) (ociannouncements.Page, error) { return ociannouncements.Page{}, nil }, get: func(context.Context, string) (notices.Notice, error) {
				return notices.Notice{}, &ociannouncements.Error{Code: ociannouncements.Service, Status: 404}
			}}
		} else {
			changed := n.Snapshot()
			changed.Summary = chain("new revision")
			source := f.source
			r.collector.announcements = announcementDetails{page: source.page, get: func(context.Context, string) (notices.Notice, error) { return normalizeCandidate(t, changed), nil }}
		}
		view, err := r.Review(leaseBudget(t), n.Key().NoticeID, noticeledger.ApproveRevision, n.Key().Digest)
		want := "source_changed"
		if unavailable {
			want = "source_unavailable"
		}
		if err != nil || view.Blocker != want || view.Fingerprint != "" || view.Saved.Snapshot().NoticeID != n.Key().NoticeID {
			t.Fatal("wrong blocked source evidence", view.Blocker, err)
		}
	}
}
