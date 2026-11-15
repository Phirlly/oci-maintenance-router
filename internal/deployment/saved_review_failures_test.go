package deployment

import (
	"context"
	"errors"
	"strings"
	"testing"

	"oci-maintenance-router/internal/ocistorage"
)

func TestSavedReviewDoesNotConcealUnreadableOrDeletedEvidence(t *testing.T) {
	for _, kind := range []string{"notice-missing", "notice-corrupt", "notice-foreign", "notice-denied", "notice-etag", "issues-corrupt", "issues-foreign", "issues-denied", "issues-deleted", "history-denied"} {
		t.Run(kind, func(t *testing.T) {
			f := newSavedReviewFixture(t, "ocid1.announcement.oc1..example", 1, true)
			f.preparationIssue(t, 0)
			prefix, want := "notices/", "review_notice_read"
			if strings.HasPrefix(kind, "issues") || kind == "history-denied" {
				prefix, want = "review/preparation/", "review_issues_read"
			}
			for name, obj := range f.objects.objects {
				if !strings.HasPrefix(name, prefix) {
					continue
				}
				switch {
				case strings.HasSuffix(kind, "missing"), strings.HasSuffix(kind, "deleted"), kind == "history-denied":
					delete(f.objects.objects, name)
				case strings.HasSuffix(kind, "corrupt"):
					obj.Body = []byte("private-provider-error")
					f.objects.objects[name] = obj
				case strings.HasSuffix(kind, "foreign"):
					obj.Body = []byte(strings.ReplaceAll(string(obj.Body), f.environment, strings.Repeat("f", 64)))
					f.objects.objects[name] = obj
				case strings.HasSuffix(kind, "etag"):
					obj.ETag = ""
					f.objects.objects[name] = obj
				}
			}
			f.objects.before = func(_ context.Context, method, name string) error {
				if method == "GET" && strings.HasPrefix(name, prefix) && strings.HasSuffix(kind, "denied") && kind != "history-denied" {
					return &ocistorage.Error{Code: ocistorage.Service, Status: 403}
				}
				if method == "HISTORY" && kind == "history-denied" {
					return errors.New("private-provider-error")
				}
				return nil
			}
			got := invokeSavedReview(t, f.h, f.notice.Key().NoticeID, "")
			if got.Status != "not-ready" || got.Code != want || got.Review != nil {
				t.Fatalf("bad evidence treated as successful review: %+v", got)
			}
		})
	}
}

func TestSavedReviewDiscardsPageAfterFinalGuardOrCleanupFailure(t *testing.T) {
	for _, kind := range []string{"configuration", "catalog", "ownership", "cleanup"} {
		t.Run(kind, func(t *testing.T) {
			f := newSavedReviewFixture(t, "ocid1.announcement.oc1..example", 1, false)
			read := false
			f.objects.before = func(_ context.Context, method, name string) error {
				if method == "GET" && strings.HasPrefix(name, "notices/") {
					read = true
				}
				if read {
					switch kind {
					case "configuration":
						f.objects.pinned.config = append(f.objects.pinned.config, ' ')
					case "catalog":
						f.objects.pinned.catalog = append(f.objects.pinned.catalog, ' ')
					case "ownership":
						if method == "GET" && name == "control/lease.json" {
							return errors.New("private-owner-failure")
						}
					case "cleanup":
						if method == "REPLACE" && name == "control/lease.json" {
							return errors.New("private-release-failure")
						}
					}
				}
				return nil
			}
			got := invokeSavedReview(t, f.h, f.notice.Key().NoticeID, "")
			if got.Status != "not-ready" || got.Review != nil {
				t.Fatalf("unguarded page retained: %+v", got)
			}
			if kind == "cleanup" && got.CleanupCode != "release_failed" || kind == "ownership" && got.Code != "ownership_failed" || (kind == "configuration" || kind == "catalog") && got.Code != "configuration_changed" {
				t.Fatalf("wrong guard failure: %+v", got)
			}
		})
	}
}
