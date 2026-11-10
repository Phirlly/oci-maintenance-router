package reconciliation

import (
	"context"
	"errors"
	"testing"
	"time"

	"oci-maintenance-router/internal/noticeledger"
	"oci-maintenance-router/internal/notices"
)

func TestCollectionCancellationWhileWaitingPreventsSavedRead(t *testing.T) {
	n := candidateNotice(t, 1, nil, true)
	r := candidateRecord(t, n)
	a, s := detailSources(t, []notices.Notice{n}, []noticeledger.Record{r})
	s.load = func(context.Context, string) (noticeledger.Record, error) {
		t.Fatal("saved read after cancellation")
		return r, nil
	}
	ctx, cancel := context.WithCancel(budget(t))
	defer cancel()
	waits := 0
	wait := func(ctx context.Context, delay time.Duration) error {
		waits++
		if waits == 3 {
			cancel()
			return pause(ctx, delay)
		}
		return nil
	}
	got, err := collector(t, a, s).collect(ctx, wait)
	wantCollectionFailure(t, got, err, Canceled, SavedNotices, 0)
	if waits != 3 || !errors.Is(err, context.Canceled) {
		t.Fatal("wrong cancellation boundary")
	}
}

func TestCollectionDeadlineDuringSavedRead(t *testing.T) {
	r := candidateRecord(t, candidateNotice(t, 1, nil, true))
	a, s := detailSources(t, nil, []noticeledger.Record{r})
	s.load = func(ctx context.Context, _ string) (noticeledger.Record, error) {
		<-ctx.Done()
		return r, errors.New("private")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	got, err := collector(t, a, s).collect(ctx, noPause)
	wantCollectionFailure(t, got, err, DeadlineExceeded, SavedNotices, 0)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("lost deadline identity")
	}
}
