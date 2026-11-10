package reconciliation

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"oci-maintenance-router/internal/noticeledger"
	"oci-maintenance-router/internal/notices"
	"oci-maintenance-router/internal/ociannouncements"
	"oci-maintenance-router/internal/ocistorage"
)

func TestCollectionSharesDeadlineAndPacingAcrossEveryStage(t *testing.T) {
	n := candidateNotice(t, 1, nil, true)
	r := candidateRecord(t, n)
	for _, duration := range []time.Duration{time.Minute, 5 * time.Minute} {
		ctx, cancel := context.WithTimeout(context.Background(), duration)
		var first time.Time
		var order []string
		check := func(ctx context.Context, step string) {
			deadline, ok := ctx.Deadline()
			if !ok {
				t.Fatal("missing deadline")
			}
			if first.IsZero() {
				first = deadline
			} else if !first.Equal(deadline) {
				t.Fatal("renewed deadline")
			}
			order = append(order, step)
		}
		a := announcementDetails{page: func(ctx context.Context, _ string) (ociannouncements.Page, error) {
			check(ctx, "list current")
			return ociannouncements.Page{IDs: []string{n.Key().NoticeID}}, nil
		}, get: func(ctx context.Context, _ string) (notices.Notice, error) { check(ctx, "get"); return n, nil }}
		s := savedDetails{page: func(ctx context.Context, _ string) (ocistorage.NoticePage, error) {
			check(ctx, "list saved")
			return ocistorage.NoticePage{Names: []string{recordName(r)}}, nil
		}, load: func(ctx context.Context, _ string) (noticeledger.Record, error) { check(ctx, "load"); return r, nil }}
		wait := func(ctx context.Context, delay time.Duration) error {
			if delay != 100*time.Millisecond {
				t.Fatal("wrong pacing")
			}
			check(ctx, "wait")
			return nil
		}
		before := time.Now()
		got, err := collector(t, a, s).collect(ctx, wait)
		cancel()
		if err != nil || len(got.Current) != 1 || len(got.Saved) != 1 {
			t.Fatal("collection failed", err)
		}
		if !slices.Equal(order, []string{"list current", "wait", "list saved", "wait", "get", "wait", "load"}) {
			t.Fatalf("wrong stages: %v", order)
		}
		if duration == time.Minute {
			parent, _ := ctx.Deadline()
			if !parent.Equal(first) {
				t.Fatal("extended parent")
			}
		} else if first.Before(before.Add(120*time.Second)) || first.After(time.Now().Add(120*time.Second)) {
			t.Fatal("wrong cap")
		}
	}
}

func TestCollectionRejectsSuccessfulDetailsAfterCancellation(t *testing.T) {
	n := candidateNotice(t, 1, nil, true)
	r := candidateRecord(t, n)
	for _, source := range []Source{Announcements, SavedNotices} {
		ctx, cancel := context.WithCancel(budget(t))
		a, s := detailSources(t, []notices.Notice{n}, []noticeledger.Record{r})
		if source == Announcements {
			a.get = func(context.Context, string) (notices.Notice, error) { cancel(); return n, nil }
			s.load = func(context.Context, string) (noticeledger.Record, error) {
				t.Fatal("read after canceled Get")
				return r, nil
			}
		} else {
			s.load = func(context.Context, string) (noticeledger.Record, error) { cancel(); return r, nil }
		}
		got, err := collector(t, a, s).collect(ctx, noPause)
		cancel()
		wantCollectionFailure(t, got, err, Canceled, source, 0)
		if !errors.Is(err, context.Canceled) {
			t.Fatal("lost cancellation")
		}
	}
}

func TestCollectionDeadlineDuringDetailRead(t *testing.T) {
	n := candidateNotice(t, 1, nil, true)
	a, s := detailSources(t, []notices.Notice{n}, nil)
	a.get = func(ctx context.Context, _ string) (notices.Notice, error) {
		<-ctx.Done()
		return n, errors.New("private")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	got, err := collector(t, a, s).collect(ctx, noPause)
	wantCollectionFailure(t, got, err, DeadlineExceeded, Announcements, 0)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("lost deadline identity")
	}
}

func TestCollectionWaitFailurePreventsDetailDispatch(t *testing.T) {
	n := candidateNotice(t, 1, nil, true)
	for _, canceled := range []bool{false, true} {
		a, s := detailSources(t, []notices.Notice{n}, nil)
		a.get = func(context.Context, string) (notices.Notice, error) { t.Fatal("late Get"); return n, nil }
		ctx, cancel := context.WithCancel(budget(t))
		waits := 0
		wait := func(context.Context, time.Duration) error {
			waits++
			if waits == 2 {
				if canceled {
					cancel()
				}
				return errors.New("private wait")
			}
			return nil
		}
		got, err := collector(t, a, s).collect(ctx, wait)
		cancel()
		code := ReadFailed
		if canceled {
			code = Canceled
		}
		wantCollectionFailure(t, got, err, code, Announcements, 0)
	}
}

func TestPublicCollectionPacesListingToDetails(t *testing.T) {
	n := candidateNotice(t, 1, nil, true)
	a, s := detailSources(t, []notices.Notice{n}, nil)
	var listed time.Time
	s.page = func(context.Context, string) (ocistorage.NoticePage, error) {
		listed = time.Now()
		return ocistorage.NoticePage{}, nil
	}
	a.get = func(context.Context, string) (notices.Notice, error) {
		if listed.IsZero() || time.Since(listed) < 100*time.Millisecond {
			t.Fatal("unpaced detail read")
		}
		return n, nil
	}
	got, err := collector(t, a, s).Collect(budget(t))
	if err != nil || len(got.Current) != 1 {
		t.Fatal("public collection failed", err)
	}
}
