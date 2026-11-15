package deployment

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"oci-maintenance-router/internal/ocistorage"
)

func TestSavedListRejectsChangedReplayAndContext(t *testing.T) {
	for _, kind := range []string{"membership", "next", "configuration", "catalog", "release", "environment"} {
		t.Run(kind, func(t *testing.T) {
			f, objects, names, _ := newSavedListFixture(t, 12)
			first := invokeSavedList(t, f.h, "")
			cursor := first.SavedNotices.NextCursor
			switch kind {
			case "membership":
				objects.pages[""] = ocistorage.NoticePage{Names: names[:11]}
			case "next":
				objects.pages[""] = ocistorage.NoticePage{Names: names, NextStartWith: names[11]}
			case "configuration":
				f.s = configureOperational(t, f.h, f.s, f.objects, func(v map[string]any) { v["mode"] = "PREVIEW" })
			case "catalog":
				f.objects.pinned.catalog = append(f.objects.pinned.catalog, ' ')
				f.s.CatalogSHA256 = digest(f.objects.pinned.catalog)
			case "release":
				f.s.ReleaseSHA256 = strings.Repeat("f", 64)
			case "environment":
				c, err := parseListingCursor(cursor)
				if err != nil {
					t.Fatal(err)
				}
				c.Context = strings.Repeat("f", 64)
				cursor = encodeListingCursor(c)
			}
			raw, _ := json.Marshal(f.s)
			f.h.getenv = func(string) string { return string(raw) }
			got := invokeSavedList(t, f.h, cursor)
			if got.Status != "not-ready" || got.Code != "listing_changed" || got.SavedNotices != nil {
				t.Fatalf("stale cursor accepted: %+v", got)
			}
			if kind != "membership" && kind != "next" && len(objects.starts) != 1 {
				t.Fatal("context mismatch read storage page")
			}
		})
	}
}

func TestSavedListIdentityDoesNotPromiseUnchangedRecordContent(t *testing.T) {
	f, _, _, _ := newSavedListFixture(t, 12)
	first := invokeSavedList(t, f.h, "")
	next, err := f.observer.Observe(f.record, f.notice, false, f.lookups, reviewTime().Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	f.save(t, next)
	got := invokeSavedList(t, f.h, first.SavedNotices.NextCursor)
	if got.Status != "listed" || len(got.SavedNotices.NoticeIDs) != 2 {
		t.Fatal(got)
	}
}

func TestSavedListRejectsOutOfRangeReplayOffset(t *testing.T) {
	f, _, _, _ := newSavedListFixture(t, 11)
	a := invokeSavedList(t, f.h, "")
	c, err := parseListingCursor(a.SavedNotices.NextCursor)
	if err != nil {
		t.Fatal(err)
	}
	c.Offset = 11
	got := invokeSavedList(t, f.h, encodeListingCursor(c))
	if got.Code != "listing_cursor_invalid" || got.SavedNotices != nil {
		t.Fatal(got)
	}
}
