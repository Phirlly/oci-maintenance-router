package deployment

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestSavedListNeverReturnsPartialIDsOnReadFailure(t *testing.T) {
	for _, kind := range []string{"listing", "deleted", "corrupt", "foreign", "path", "etag", "oversized", "denied"} {
		t.Run(kind, func(t *testing.T) {
			f, _, names, _ := newSavedListFixture(t, 2)
			name := names[1] // Failure after a valid ID must discard that ID too.
			obj := f.objects.objects[name]
			switch kind {
			case "deleted":
				delete(f.objects.objects, name)
			case "corrupt":
				obj.Body = []byte("private-invalid-record")
			case "foreign":
				obj.Body = []byte(strings.ReplaceAll(string(obj.Body), f.environment, strings.Repeat("f", 64)))
			case "path":
				obj.Body = f.objects.objects[names[0]].Body
			case "etag":
				obj.ETag = ""
			case "oversized":
				obj.Body = make([]byte, (1<<20)+1)
			}
			if kind != "deleted" {
				f.objects.objects[name] = obj
			}
			f.objects.before = func(_ context.Context, method, key string) error {
				if kind == "listing" && method == "LIST" || kind == "denied" && method == "GET" && key == name {
					return errors.New("private-provider-error")
				}
				return nil
			}
			got := invokeSavedList(t, f.h, "")
			want := "listing_notice_read"
			if kind == "listing" {
				want = "listing_read"
			}
			if got.Status != "not-ready" || got.Code != want || got.SavedNotices != nil {
				t.Fatalf("failed page exposed partial evidence: %+v", got)
			}
		})
	}
}

func TestSavedListDropsPayloadAfterFinalGuardOrCleanupFailure(t *testing.T) {
	for _, kind := range []string{"configuration", "catalog", "ownership", "cleanup"} {
		t.Run(kind, func(t *testing.T) {
			f, _, _, _ := newSavedListFixture(t, 1)
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
			got := invokeSavedList(t, f.h, "")
			if got.Status != "not-ready" || got.SavedNotices != nil {
				t.Fatalf("unguarded IDs retained: %+v", got)
			}
			if kind == "cleanup" && got.CleanupCode != "release_failed" || kind == "ownership" && got.Code != "ownership_failed" || (kind == "configuration" || kind == "catalog") && got.Code != "configuration_changed" {
				t.Fatalf("wrong guard failure: %+v", got)
			}
		})
	}
}
