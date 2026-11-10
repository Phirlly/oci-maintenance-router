package reconciliation

import (
	"context"
	"testing"

	"oci-maintenance-router/internal/noticeledger"
	"oci-maintenance-router/internal/notices"
	"oci-maintenance-router/internal/ociannouncements"
	"oci-maintenance-router/internal/ocistorage"
)

func TestCollectionPreservesChangedDuplicatesAndSavedChainRelevance(t *testing.T) {
	ns := []notices.Notice{candidateNotice(t, 1, chain("history"), false), candidateNotice(t, 1, chain("changed"), false), candidateNotice(t, 2, nil, false)}
	r := candidateRecord(t, candidateNotice(t, 3, chain("history"), true))
	a, s := detailSources(t, ns, []noticeledger.Record{r, r})
	got, err := collector(t, a, s).collect(budget(t), noPause)
	if err != nil || len(got.Current) != 3 || len(got.Saved) != 2 {
		t.Fatalf("collection=%+v err=%v", got, err)
	}
	for i, n := range ns {
		if got.Current[i].Revision != n.Key() || got.Current[i].KnownCandidate != (i < 2) {
			t.Fatalf("wrong current match %d: %+v", i, got.Current[i])
		}
	}
	for _, saved := range got.Saved {
		if saved.ObjectName != recordName(r) || saved.NoticeID != noticeID(3) {
			t.Fatal("saved identity/order lost")
		}
	}
}

func TestCollectionReadsEmptyContinuationAndBothEmptySources(t *testing.T) {
	aCalls, sCalls := 0, 0
	a := announcementDetails{page: func(_ context.Context, cursor string) (ociannouncements.Page, error) {
		aCalls++
		if aCalls == 1 {
			return ociannouncements.Page{NextPage: "next"}, nil
		}
		if cursor != "next" {
			t.Fatal("wrong continuation")
		}
		return ociannouncements.Page{}, nil
	}}
	s := savedDetails{page: func(context.Context, string) (ocistorage.NoticePage, error) {
		sCalls++
		return ocistorage.NoticePage{}, nil
	}}
	got, err := collector(t, a, s).collect(budget(t), noPause)
	if err != nil || len(got.Current) != 0 || len(got.Saved) != 0 || aCalls != 2 || sCalls != 1 {
		t.Fatalf("empty traversal: %+v %v calls=%d/%d", got, err, aCalls, sCalls)
	}
}

func TestCollectionRetainsSavedOnlyReferences(t *testing.T) {
	r := candidateRecord(t, candidateNotice(t, 5, nil, true))
	a, s := detailSources(t, nil, []noticeledger.Record{r})
	got, err := collector(t, a, s).collect(budget(t), noPause)
	if err != nil || len(got.Current) != 0 || len(got.Saved) != 1 || got.Saved[0].NoticeID != noticeID(5) {
		t.Fatalf("saved-only work lost: %+v %v", got, err)
	}
}
