package deployment

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"oci-maintenance-router/internal/ocistorage"
)

func TestSavedListByteDeferralPreservesUnconsumedID(t *testing.T) {
	f, objects, _, _ := newSavedListFixture(t, 0)
	ids := map[string]bool{}
	for _, suffix := range []string{"a", "b", "c"} {
		id := "ocid1.announcement.oc1.." + strings.Repeat(suffix, 4096-len("ocid1.announcement.oc1.."))
		addListedNotice(t, f, id)
		ids[id] = true
	}
	page, err := f.objects.ListNoticePage(operationalContext(t), "")
	if err != nil {
		t.Fatal(err)
	}
	objects.pages[""] = page
	reads, seen, cursor := 0, map[string]bool{}, ""
	f.objects.before = func(_ context.Context, method, name string) error {
		if method == "GET" && strings.HasPrefix(name, "notices/") {
			reads++
		}
		return nil
	}
	for i := 0; i < 3; i++ {
		reads = 0
		got := invokeSavedList(t, f.h, cursor)
		if got.Status != "listed" || len(got.SavedNotices.NoticeIDs) != 1 || reads > 2 {
			t.Fatalf("byte limit failed: %+v reads=%d", got, reads)
		}
		id := got.SavedNotices.NoticeIDs[0]
		if !ids[id] || seen[id] {
			t.Fatal("truncated, repeated or unrecognized ID")
		}
		seen[id] = true
		cursor = got.SavedNotices.NextCursor
		if (i == 2) != (cursor == "") {
			t.Fatal("wrong continuation")
		}
	}
}

func TestSavedListRefusesUnsupportedIDWithoutTruncation(t *testing.T) {
	f, objects, _, _ := newSavedListFixture(t, 1)
	addListedNotice(t, f, "ocid1.announcement.oc1.."+strings.Repeat("a", 4096))
	page, err := f.objects.ListNoticePage(operationalContext(t), "")
	if err != nil {
		t.Fatal(err)
	}
	objects.pages[""] = page
	got := invokeSavedList(t, f.h, "")
	if got.Status != "not-ready" || got.Code != "listing_notice_invalid" || got.SavedNotices != nil {
		t.Fatal(got)
	}
}

func TestSavedListBoundsServicePageChainAndRejectsSelfLoop(t *testing.T) {
	for _, kind := range []string{"terminal", "limit", "self-loop"} {
		t.Run(kind, func(t *testing.T) {
			f, objects, _, _ := newSavedListFixture(t, 0)
			start := ""
			for i := 0; i < 100; i++ {
				next := fmt.Sprintf("notices/%064x.json", i+1)
				if i == 99 && kind == "terminal" {
					next = ""
				}
				if i == 1 && kind == "self-loop" {
					next = start
				}
				objects.pages[start] = ocistorage.NoticePage{NextStartWith: next}
				if kind == "self-loop" && i == 1 {
					break
				}
				start = next
			}
			cursor := ""
			for i := 0; i < 100; i++ {
				got := invokeSavedList(t, f.h, cursor)
				if kind == "self-loop" && i == 1 || kind == "limit" && i == 99 {
					want := "listing_limit"
					if kind == "self-loop" {
						want = "listing_cycle"
					}
					if got.Code != want || got.SavedNotices != nil {
						t.Fatal(got)
					}
					break
				}
				if got.Status != "listed" || len(got.SavedNotices.NoticeIDs) != 0 {
					t.Fatal(got)
				}
				cursor = got.SavedNotices.NextCursor
				if (i == 99) != (cursor == "") {
					t.Fatal("incorrect page chain bound")
				}
			}
		})
	}
}
