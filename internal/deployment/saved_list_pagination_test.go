package deployment

import (
	"context"
	"reflect"
	"slices"
	"strings"
	"testing"

	"oci-maintenance-router/internal/ocistorage"
)

func TestSavedListTraversesSubpagesThenExactServiceContinuation(t *testing.T) {
	f, objects, names, ids := newSavedListFixture(t, 25)
	objects.pages[""] = ocistorage.NoticePage{Names: names[:21], NextStartWith: names[21]}
	objects.pages[names[21]] = ocistorage.NoticePage{Names: names[21:]}
	want := []string{}
	for _, name := range names {
		want = append(want, ids[name])
	}
	gotIDs, cursor, reads := []string{}, "", 0
	f.objects.before = func(_ context.Context, method, name string) error {
		if method == "GET" && strings.HasPrefix(name, "notices/") {
			reads++
		}
		return nil
	}
	for i := 0; i < 4; i++ {
		reads = 0
		got := invokeSavedList(t, f.h, cursor)
		if got.Status != "listed" || got.SavedNotices == nil || reads > 10 || len(objects.starts) != i+1 {
			t.Fatalf("bounded listing failed: %+v reads=%d", got, reads)
		}
		gotIDs = append(gotIDs, got.SavedNotices.NoticeIDs...)
		cursor = got.SavedNotices.NextCursor
		if (i == 3) != (cursor == "") {
			t.Fatal("incorrect terminal page")
		}
	}
	if !reflect.DeepEqual(gotIDs, want) || !reflect.DeepEqual(objects.starts, []string{"", "", "", names[21]}) {
		t.Fatalf("omitted/duplicated ID or incorrect inclusive start: %v %v", gotIDs, objects.starts)
	}
}

func TestSavedListReplaysMembershipWithoutAssumingArrayOrder(t *testing.T) {
	f, objects, names, _ := newSavedListFixture(t, 12)
	first := invokeSavedList(t, f.h, "")
	reordered := slices.Clone(names)
	slices.Reverse(reordered)
	objects.pages[""] = ocistorage.NoticePage{Names: reordered}
	a := invokeSavedList(t, f.h, first.SavedNotices.NextCursor)
	b := invokeSavedList(t, f.h, first.SavedNotices.NextCursor)
	if a.Status != "listed" || !reflect.DeepEqual(a.SavedNotices, b.SavedNotices) || len(a.SavedNotices.NoticeIDs) != 2 || a.SavedNotices.NextCursor != "" {
		t.Fatalf("reordered stable page rejected: %+v", a)
	}
	if !reflect.DeepEqual(objects.pages[""].Names, reordered) {
		t.Fatal("mutated service page")
	}
}

func TestSavedListEmptyContinuationAndTerminalPage(t *testing.T) {
	f, objects, names, ids := newSavedListFixture(t, 1)
	objects.pages[""] = ocistorage.NoticePage{NextStartWith: names[0]}
	objects.pages[names[0]] = ocistorage.NoticePage{Names: names}
	a := invokeSavedList(t, f.h, "")
	if a.Status != "listed" || len(a.SavedNotices.NoticeIDs) != 0 || a.SavedNotices.NextCursor == "" {
		t.Fatal(a)
	}
	b := invokeSavedList(t, f.h, a.SavedNotices.NextCursor)
	if b.Status != "listed" || !reflect.DeepEqual(b.SavedNotices.NoticeIDs, []string{ids[names[0]]}) || b.SavedNotices.NextCursor != "" {
		t.Fatal(b)
	}
	objects.pages[""] = ocistorage.NoticePage{}
	c := invokeSavedList(t, f.h, "")
	if c.Status != "listed" || len(c.SavedNotices.NoticeIDs) != 0 || c.SavedNotices.NextCursor != "" {
		t.Fatal(c)
	}
}
