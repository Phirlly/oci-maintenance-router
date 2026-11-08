package ocistorage

import (
	"sync"
	"testing"

	"oci-maintenance-router/internal/delivery"
)

func TestCompetingWholeNoticeUpdatesHaveOneWinner(t *testing.T) {
	c, writes := conditionalServer(t, false)
	r := noticeState(t)
	s := noticeStore(t, c, r)
	if _, e := s.Create(budget(t), r); e != nil {
		t.Fatal(e)
	}
	a, e := s.Load(budget(t), stateNoticeID)
	if e != nil {
		t.Fatal(e)
	}
	b, e := s.Load(budget(t), stateNoticeID)
	if e != nil {
		t.Fatal(e)
	}
	ctx := budget(t)
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for _, previous := range []StoredNotice{a, b} {
		next := holdState(t, previous.Record(), delivery.ReasonOperator)
		wg.Go(func() { _, e := s.Save(ctx, previous, next); results <- e })
	}
	wg.Wait()
	close(results)
	winners := 0
	for e := range results {
		if e == nil {
			winners++
		} else {
			wantError(t, e, Conflict, 412, false)
		}
	}
	if winners != 1 || writes.Load() != 3 {
		t.Fatal("whole-notice conflict overwritten or replayed")
	}
}

func TestLostWholeNoticeWriteReturnsNoHandleAndRereadRecovers(t *testing.T) {
	for _, drop := range []int{1, 2} {
		c, writes := conditionalServerAt(t, drop)
		r := noticeState(t)
		s := noticeStore(t, c, r)
		got, e := s.Create(budget(t), r)
		if drop == 2 {
			if e != nil {
				t.Fatal(e)
			}
			r = holdState(t, got.Record(), delivery.ReasonOperator)
			got, e = s.Save(budget(t), got, r)
		}
		wantError(t, e, Transport, 0, true)
		if got.Record().Snapshot().NoticeID != "" || writes.Load() != int32(drop) {
			t.Fatal("uncertain write returned handle or retried")
		}
		loaded, e := s.Load(budget(t), stateNoticeID)
		if e != nil || loaded.Record().Snapshot().HoldReason != r.Snapshot().HoldReason {
			t.Fatal("reread failed to recover saved whole state")
		}
	}
}
