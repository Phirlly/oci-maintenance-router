package deployment

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"oci-maintenance-router/internal/noticeledger"
)

func TestSavedReviewPagesMaximumIDWithoutMissingOrDuplicatingItems(t *testing.T) {
	id := "ocid1.announcement.oc1.." + strings.Repeat("a", 4096-len("ocid1.announcement.oc1.."))
	f := newSavedReviewFixture(t, id, 80, false)
	before := businessObjects(f.objects)
	cursor := ""
	all := []SavedReviewItem{}
	for page := 0; page < 80; page++ {
		got := invokeSavedReview(t, f.h, id, cursor)
		if got.Status != "reviewed" || got.Review == nil || got.Review.TotalItems != 80 || len(got.Review.Items) == 0 || len(got.Review.Items) > 32 {
			t.Fatalf("missing bounded page: %+v", got)
		}
		body, _ := json.Marshal(got.Review)
		if len(body) > 6500 {
			t.Fatal("page exceeded byte budget")
		}
		repeated := invokeSavedReview(t, f.h, id, cursor)
		if !reflect.DeepEqual(got, repeated) {
			t.Fatal("unchanged request changed projection")
		}
		all = append(all, got.Review.Items...)
		cursor = got.Review.NextCursor
		if cursor == "" {
			break
		}
	}
	view, err := f.record.Review(noticeledger.PreparationIssues{}, f.s.ConfigurationSHA256)
	if err != nil {
		t.Fatal(err)
	}
	if cursor != "" || !reflect.DeepEqual(all, savedReviewItems(view)) || !reflect.DeepEqual(before, businessObjects(f.objects)) {
		t.Fatal("pagination lost, duplicated or mutated saved work")
	}
}

func TestSavedReviewContinuationRejectsChangedBindings(t *testing.T) {
	for _, kind := range []string{"notice", "issues", "configuration", "catalog", "release"} {
		t.Run(kind, func(t *testing.T) {
			f := newSavedReviewFixture(t, "ocid1.announcement.oc1..example", 80, kind == "issues")
			if kind == "issues" {
				for i := 0; i < 80; i++ {
					f.preparationIssue(t, i)
				}
			}
			first := invokeSavedReview(t, f.h, f.notice.Key().NoticeID, "")
			if first.Review == nil || first.Review.NextCursor == "" {
				t.Fatal("fixture needs multiple pages")
			}
			switch kind {
			case "notice", "issues":
				prefix := "notices/"
				if kind == "issues" {
					prefix = "review/preparation/"
				}
				for name, obj := range f.objects.objects {
					if strings.HasPrefix(name, prefix) {
						obj.Body = []byte(strings.ReplaceAll(string(obj.Body), "2026-01-01T00:00:00.000Z", "2026-01-01T00:00:01.000Z"))
						f.objects.objects[name] = obj
					}
				}
			case "configuration":
				f.s = configureOperational(t, f.h, f.s, f.objects, func(v map[string]any) { v["mode"] = "PREVIEW" })
			case "catalog":
				f.objects.pinned.catalog = append(f.objects.pinned.catalog, ' ')
				f.s.CatalogSHA256 = digestFixture(f.objects.pinned.catalog)
			case "release":
				f.s.ReleaseSHA256 = strings.Repeat("f", 64)
			}
			body, _ := json.Marshal(f.s)
			f.h.getenv = func(string) string { return string(body) }
			got := invokeSavedReview(t, f.h, f.notice.Key().NoticeID, first.Review.NextCursor)
			if got.Status != "not-ready" || got.Code != "review_changed" || got.Review != nil {
				t.Fatalf("stale page accepted: %+v", got)
			}
		})
	}
}

func TestSavedReviewRejectsInvalidCursorAndOversizedPage(t *testing.T) {
	binding := strings.Repeat("a", 64)
	view := noticeledger.SavedReview{Issues: []noticeledger.SavedReviewIssue{{Kind: "RESOURCE", Code: "lookup_missing"}}}
	for _, cursor := range []string{"bad", binding + ":0", binding + ":01", binding + ":1000000", strings.Repeat("b", 64) + ":1", binding + ":1"} {
		if page, err := savedReviewPage(noticeledger.Snapshot{}, view, binding, cursor); err == nil || page != nil {
			t.Fatalf("invalid cursor accepted: %q", cursor)
		}
	}
	view.Issues[0].Code = strings.Repeat("x", 6500)
	if page, err := savedReviewPage(noticeledger.Snapshot{}, view, binding, ""); err == nil || page != nil {
		t.Fatal("oversized item silently truncated")
	}
	page, err := savedReviewPage(noticeledger.Snapshot{}, noticeledger.SavedReview{}, binding, "")
	if err != nil || len(page.Items) != 0 || page.NextCursor != "" {
		t.Fatal("empty saved view did not terminate", err)
	}
}
