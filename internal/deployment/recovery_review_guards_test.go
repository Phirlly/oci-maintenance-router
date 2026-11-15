package deployment

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestRecoveryCatalogDoesNotConcealFailedReadsOrFinalGuards(t *testing.T) {
	for _, kind := range []string{"missing", "corrupt", "denied", "configuration", "cleanup", "ownership"} {
		t.Run(kind, func(t *testing.T) {
			f := reviewLiveFixture(t)
			read := false
			for name, obj := range f.objects.objects {
				if strings.HasPrefix(name, "notices/") {
					if kind == "missing" {
						delete(f.objects.objects, name)
					}
					if kind == "corrupt" {
						obj.Body = []byte("private")
						f.objects.objects[name] = obj
					}
				}
			}
			f.objects.before = func(_ context.Context, method, name string) error {
				if method == "GET" && strings.HasPrefix(name, "notices/") {
					read = true
					if kind == "denied" {
						return errors.New("private")
					}
				}
				if read {
					if kind == "configuration" {
						f.objects.pinned.config = append(f.objects.pinned.config, ' ')
					}
					if name == "control/lease.json" && (kind == "cleanup" && method == "REPLACE" || kind == "ownership" && method == "GET") {
						return errors.New("private")
					}
				}
				return nil
			}
			got := closureResponse(t, f.h, recoveryReviewRequest(f.notice.Key().NoticeID, "", "", ""))
			if got["status"] != "not-ready" || got["recoveryReview"] != nil {
				t.Fatal("failed review disclosed code", got)
			}
			want := "recovery_notice_read"
			if kind == "configuration" {
				want = "configuration_changed"
			}
			if kind == "ownership" {
				want = "ownership_failed"
			}
			if kind == "cleanup" {
				if got["cleanupCode"] != "release_failed" {
					t.Fatal(got)
				}
			} else if got["code"] != want {
				t.Fatal("wrong failure", got)
			}
		})
	}
}
