package reconciliation

import (
	"context"
	"reflect"
	"testing"
	"time"

	"oci-maintenance-router/internal/ociannouncements"
	"oci-maintenance-router/internal/ocistorage"
)

func TestEnumerationPreservesOccurrencesAndEmptyContinuations(t *testing.T) {
	aCalls, sCalls, waits := []string{}, []string{}, 0
	token := " +/%?é= "
	a := announcementPages(func(_ context.Context, cursor string) (ociannouncements.Page, error) {
		aCalls = append(aCalls, cursor)
		switch len(aCalls) {
		case 1:
			return ociannouncements.Page{NextPage: token}, nil
		case 2:
			return ociannouncements.Page{IDs: []string{noticeID(2), noticeID(1), noticeID(2)}, NextPage: "last"}, nil
		default:
			return ociannouncements.Page{IDs: []string{noticeID(1)}}, nil
		}
	})
	s := savedPages(func(_ context.Context, cursor string) (ocistorage.NoticePage, error) {
		sCalls = append(sCalls, cursor)
		if len(aCalls) != 3 {
			t.Fatal("saved traversal started before source listing ended")
		}
		switch len(sCalls) {
		case 1:
			return ocistorage.NoticePage{Names: []string{objectName(2)}, NextStartWith: objectName(1)}, nil
		case 2:
			return ocistorage.NoticePage{NextStartWith: objectName(3)}, nil
		default:
			return ocistorage.NoticePage{Names: []string{objectName(2), objectName(1)}}, nil
		}
	})
	wait := func(_ context.Context, d time.Duration) error {
		waits++
		if d != 100*time.Millisecond {
			t.Fatal("incorrect pacing", d)
		}
		return nil
	}
	got, err := enumerate(budget(t), a, s, wait)
	want := Inventory{[]string{noticeID(2), noticeID(1), noticeID(2), noticeID(1)}, []string{objectName(2), objectName(2), objectName(1)}}
	if err != nil || !reflect.DeepEqual(got, want) || waits != 5 ||
		!reflect.DeepEqual(aCalls, []string{"", token, "last"}) ||
		!reflect.DeepEqual(sCalls, []string{"", objectName(1), objectName(3)}) {
		t.Fatalf("incorrect complete traversal: %+v %v calls=%v/%v waits=%d", got, err, aCalls, sCalls, waits)
	}
}

func TestEnumerationOwnsResultsAndStartsFresh(t *testing.T) {
	aIDs, sNames := []string{noticeID(1)}, []string{objectName(1)}
	a := announcementPages(func(_ context.Context, cursor string) (ociannouncements.Page, error) {
		if cursor != "" {
			t.Fatal("resumed old source cursor")
		}
		return ociannouncements.Page{IDs: aIDs}, nil
	})
	s := savedPages(func(_ context.Context, cursor string) (ocistorage.NoticePage, error) {
		if cursor != "" {
			t.Fatal("resumed old saved cursor")
		}
		return ocistorage.NoticePage{Names: sNames}, nil
	})
	first, err := enumerate(budget(t), a, s, noPause)
	if err != nil || len(first.AnnouncementIDs) != 1 || len(first.NoticeObjects) != 1 {
		t.Fatal("missing work", err)
	}
	first.AnnouncementIDs[0], first.NoticeObjects[0] = "caller mutation", "caller mutation"
	second, err := enumerate(budget(t), a, s, noPause)
	aIDs[0], sNames[0] = "adapter mutation", "adapter mutation"
	if err != nil || second.AnnouncementIDs[0] != noticeID(1) || second.NoticeObjects[0] != objectName(1) {
		t.Fatal("result aliases adapter or previous result")
	}
}

func TestEmptyEnumerationStillReadsBothSources(t *testing.T) {
	count := 0
	a := announcementPages(func(context.Context, string) (ociannouncements.Page, error) {
		count++
		return ociannouncements.Page{}, nil
	})
	s := savedPages(func(context.Context, string) (ocistorage.NoticePage, error) {
		count++
		return ocistorage.NoticePage{}, nil
	})
	got, err := Enumerate(budget(t), a, s)
	if err != nil || count != 2 || len(got.AnnouncementIDs) != 0 || len(got.NoticeObjects) != 0 {
		t.Fatal("empty success must finish both lists", err, count)
	}
}
