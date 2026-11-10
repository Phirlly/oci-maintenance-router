package reconciliation

import (
	"context"
	"strings"
	"testing"

	"oci-maintenance-router/internal/noticeledger"
	"oci-maintenance-router/internal/notices"
	"oci-maintenance-router/internal/ocistorage"
)

func TestCollectionCountsAllSavedRevisionsAgainstCandidateLimit(t *testing.T) {
	n := candidateNotice(t, 0, nil, true)
	r := candidateRecord(t, n, candidateNotice(t, 0, chain("changed"), true))
	ns := make([]notices.Notice, 100)
	for i := range ns {
		ns[i] = n
	}
	a, s := detailSources(t, ns, nil)
	page, loads := 0, 0
	s.page = func(context.Context, string) (ocistorage.NoticePage, error) {
		page++
		names := make([]string, 1000)
		for i := range names {
			names[i] = recordName(r)
		}
		next := ""
		if page < 5 {
			next = objectName(page)
		}
		return ocistorage.NoticePage{Names: names, NextStartWith: next}, nil
	}
	s.load = func(context.Context, string) (noticeledger.Record, error) { loads++; return r, nil }
	got, err := collector(t, a, s).collect(budget(t), noPause)
	wantCollectionFailure(t, got, err, ItemLimit, SavedNotices, 0)
	if page != 5 || loads != 4951 {
		t.Fatalf("wrong budget stop: pages=%d loads=%d", page, loads)
	}
}

func TestCollectionCombinedTextOverflowReturnsNoWork(t *testing.T) {
	n := candidateNotice(t, 0, chain(strings.Repeat("x", 600000)), true)
	r := candidateRecord(t, n)
	for _, source := range []Source{Announcements, SavedNotices} {
		ns := []notices.Notice{n, n, n, n}
		rs := []noticeledger.Record{r, r, r}
		if source == Announcements {
			ns = append(ns, n, n, n)
			rs = nil
		}
		a, s := detailSources(t, ns, rs)
		got, err := collector(t, a, s).collect(budget(t), noPause)
		wantCollectionFailure(t, got, err, ByteLimit, source, 0)
	}
}
