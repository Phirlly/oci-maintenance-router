package reconciliation

import (
	"context"
	"strings"
	"testing"

	"oci-maintenance-router/internal/noticeledger"
	"oci-maintenance-router/internal/notices"
	"oci-maintenance-router/internal/ocistorage"
)

func TestSavedSourceSharesExactCaptureBudgetWithInitialDetails(t *testing.T) {
	bases := []notices.Notice{}
	room := 4 << 20
	for i := 1; i <= 5; i++ {
		src := candidateNotice(t, i, nil, true).Snapshot()
		src.Reference = chain("")
		n := normalizeCandidate(t, src)
		bases = append(bases, n)
		room -= captureSize(t, n)
	}
	for _, overflow := range []bool{false, true} {
		ns := []notices.Notice{}
		for i, n := range bases {
			src := n.Snapshot()
			padding := room / 5
			if i == 4 {
				padding += room % 5
				if overflow {
					padding++
				}
			}
			src.Reference = chain(strings.Repeat("x", padding))
			ns = append(ns, normalizeCandidate(t, src))
		}
		a, s := detailSources(t, ns[:4], []noticeledger.Record{candidateRecord(t, bases[4])})
		reads := 0
		a.get = func(_ context.Context, id string) (notices.Notice, error) {
			if reads >= len(ns) || id != ns[reads].Key().NoticeID {
				t.Fatal("wrong read")
			}
			n := ns[reads]
			reads++
			return n, nil
		}
		got, e := collector(t, a, s).collect(budget(t), noPause)
		if overflow {
			wantCollectionFailure(t, got, e, ByteLimit, Announcements, 0)
		} else if e != nil || len(got.Observations) != 5 {
			t.Fatal("shared exact limit rejected or saved source skipped", e)
		}
		if reads != 5 {
			t.Fatal("missing additional source read")
		}
	}
}

func TestSavedSourceFactAdmissionCannotResetCombinedLimit(t *testing.T) {
	n := candidateNotice(t, 1, nil, true)
	r := candidateRecord(t, n, candidateNotice(t, 1, chain("changed"), true))
	a, s := detailSources(t, nil, nil)
	pages, loads, gets := 0, 0, 0
	s.page = func(context.Context, string) (ocistorage.NoticePage, error) {
		pages++
		names := make([]string, 1000)
		for i := range names {
			names[i] = recordName(r)
		}
		next := ""
		if pages < 5 {
			next = objectName(pages)
		}
		return ocistorage.NoticePage{Names: names, NextStartWith: next}, nil
	}
	s.load = func(context.Context, string) (noticeledger.Record, error) { loads++; return r, nil }
	a.get = func(context.Context, string) (notices.Notice, error) { gets++; return n, nil }
	got, e := collector(t, a, s).collect(budget(t), noPause)
	wantCollectionFailure(t, got, e, ItemLimit, Announcements, 0)
	if pages != 5 || loads != 5000 || gets != 1 {
		t.Fatal("wrong combined budget boundary", pages, loads, gets)
	}
}

func TestSavedSourceFactBytesIncludeAllEarlierSavedOccurrences(t *testing.T) {
	n := candidateNotice(t, 1, chain(strings.Repeat("x", 600000)), true)
	r := candidateRecord(t, n)
	a, s := detailSources(t, nil, []noticeledger.Record{r, r, r, r, r, r})
	reads := 0
	a.get = func(context.Context, string) (notices.Notice, error) {
		reads++
		return n, nil
	}
	got, err := collector(t, a, s).collect(budget(t), noPause)
	wantCollectionFailure(t, got, err, ByteLimit, Announcements, 0)
	if reads != 1 {
		t.Fatal("wrong combined fact-byte boundary")
	}
}
