package reconciliation

import (
	"context"
	"reflect"
	"testing"

	"oci-maintenance-router/internal/noticeledger"
	"oci-maintenance-router/internal/notices"
)

func TestSavedSourceReadsEachAbsentUniqueIDInFirstSavedOrder(t *testing.T) {
	listed := candidateNotice(t, 1, nil, true)
	aNotice := candidateNotice(t, 3, nil, true)
	bNotice := candidateNotice(t, 2, nil, true)
	saved := []noticeledger.Record{candidateRecord(t, aNotice), candidateRecord(t, listed), candidateRecord(t, aNotice), candidateRecord(t, bNotice)}
	a, s := detailSources(t, []notices.Notice{listed, listed}, saved)
	expected := []notices.Notice{listed, listed, aNotice, bNotice}
	reads, loads := 0, 0
	originalLoad := s.load
	s.load = func(ctx context.Context, name string) (noticeledger.Record, error) {
		loads++
		return originalLoad(ctx, name)
	}
	a.get = func(_ context.Context, id string) (notices.Notice, error) {
		if reads >= len(expected) || id != expected[reads].Key().NoticeID {
			t.Fatal("unexpected source read/order/retry")
		}
		if reads >= 2 && loads != len(saved) {
			t.Fatal("saved-only Get before full saved stage")
		}
		n := expected[reads]
		reads++
		return n, nil
	}
	got, e := collector(t, a, s).collect(budget(t), noPause)
	if e != nil || reads != 4 || len(got.Current) != 4 || len(got.Observations) != 4 || len(got.Saved) != 4 {
		t.Fatal("missing unique saved-only current evidence", reads, e)
	}
	for i, n := range expected {
		if got.Current[i].Revision != n.Key() || got.Observations[i].Key() != n.Key() {
			t.Fatal("occurrence order changed")
		}
	}
	if got.Availability.EnvironmentID() != got.Candidates.EnvironmentID() || got.Availability.Unavailable() == nil || len(got.Availability.Unavailable()) != 0 {
		t.Fatal("successful availability missing")
	}
}

func TestSavedSourceSuccessLinksEarlierOriginalObservationBeforeClosure(t *testing.T) {
	early := candidateNotice(t, 1, chain("new-link"), false)
	snapshot := early.Snapshot()
	snapshot.Reference = chain("original")
	early = normalizeCandidate(t, snapshot)
	old := candidateNotice(t, 2, chain("old-link"), false)
	current := candidateNotice(t, 2, chain("new-link"), false)
	a, s := detailSources(t, []notices.Notice{early}, []noticeledger.Record{candidateRecord(t, old)})
	reads := 0
	a.get = func(_ context.Context, id string) (notices.Notice, error) {
		reads++
		if reads == 1 && id == early.Key().NoticeID {
			return early, nil
		}
		if reads == 2 && id == current.Key().NoticeID {
			return current, nil
		}
		t.Fatal("refetch or unexpected read")
		return notices.Notice{}, nil
	}
	got, e := collector(t, a, s).collect(budget(t), noPause)
	if e != nil || reads != 2 || len(got.Observations) != 2 || !got.Current[0].KnownCandidate {
		t.Fatal("late chain connection discarded", e)
	}
	if got.Observations[0].Key() != early.Key() || got.Observations[1].Key() != current.Key() || len(got.Candidates.Components()) != 1 || len(got.Candidates.Components()[0].Members) != 3 {
		t.Fatal("original/history membership lost")
	}
	if !reflect.DeepEqual(got.Current, got.Candidates.Current()) {
		t.Fatal("source/view mismatch")
	}
}
