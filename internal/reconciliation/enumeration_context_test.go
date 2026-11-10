package reconciliation

import (
	"context"
	"errors"
	"testing"
	"time"

	"oci-maintenance-router/internal/ociannouncements"
	"oci-maintenance-router/internal/ocistorage"
)

func TestOneDiscoveryDeadlineCoversBothSources(t *testing.T) {
	for _, duration := range []time.Duration{time.Minute, 5 * time.Minute} {
		ctx, cancel := context.WithTimeout(context.Background(), duration)
		var first time.Time
		a := announcementPages(func(ctx context.Context, _ string) (ociannouncements.Page, error) {
			first, _ = ctx.Deadline()
			return ociannouncements.Page{}, nil
		})
		s := savedPages(func(ctx context.Context, _ string) (ocistorage.NoticePage, error) {
			second, _ := ctx.Deadline()
			if !second.Equal(first) {
				t.Fatal("renewed discovery budget")
			}
			return ocistorage.NoticePage{}, nil
		})
		before := time.Now()
		_, err := enumerate(ctx, a, s, noPause)
		cancel()
		if err != nil {
			t.Fatal(err)
		}
		if duration == time.Minute {
			parent, _ := ctx.Deadline()
			if !first.Equal(parent) {
				t.Fatal("extended earlier parent deadline")
			}
		} else if first.Before(before.Add(120*time.Second)) || first.After(time.Now().Add(120*time.Second)) {
			t.Fatal("wrong discovery cap", first)
		}
	}
}

func TestCancellationRejectsEvenSuccessfulLastPage(t *testing.T) {
	for _, stage := range []Source{"", Announcements, SavedNotices} {
		ctx, cancel := context.WithCancel(budget(t))
		if stage == "" {
			cancel()
		}
		a := announcementPages(func(context.Context, string) (ociannouncements.Page, error) {
			if stage == "" {
				t.Fatal("called after cancellation")
			}
			if stage == Announcements {
				cancel()
			}
			return ociannouncements.Page{IDs: []string{noticeID(1)}}, nil
		})
		s := savedPages(func(context.Context, string) (ocistorage.NoticePage, error) {
			if stage != SavedNotices {
				t.Fatal("continued after cancellation")
			}
			cancel()
			return ocistorage.NoticePage{Names: []string{objectName(1)}}, nil
		})
		got, err := enumerate(ctx, a, s, noPause)
		cancel()
		wantFailure(t, got, err, Canceled, stage, 0)
		if !errors.Is(err, context.Canceled) {
			t.Fatal("lost cancel identity")
		}
	}
}

func TestWaitingCancellationPreventsNextRead(t *testing.T) {
	ctx, cancel := context.WithCancel(budget(t))
	defer cancel()
	waits := 0
	wait := func(ctx context.Context, d time.Duration) error { waits++; cancel(); return pause(ctx, d) }
	s := savedPages(func(context.Context, string) (ocistorage.NoticePage, error) {
		t.Fatal("called after wait cancellation")
		return ocistorage.NoticePage{}, nil
	})
	got, err := enumerate(ctx, announcementPages(emptyAnnouncements), s, wait)
	wantFailure(t, got, err, Canceled, SavedNotices, 0)
	if waits != 1 {
		t.Fatal("pacing omitted")
	}
}

func TestDeadlineDuringReadReturnsNoInventory(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	a := announcementPages(func(ctx context.Context, _ string) (ociannouncements.Page, error) {
		<-ctx.Done()
		return ociannouncements.Page{}, errors.New("private")
	})
	got, err := enumerate(ctx, a, savedPages(emptySaved), noPause)
	wantFailure(t, got, err, DeadlineExceeded, Announcements, 0)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("lost deadline identity")
	}
}

func TestPublicEnumerationUsesCancelableSpacing(t *testing.T) {
	var returnedAt time.Time
	a := announcementPages(func(context.Context, string) (ociannouncements.Page, error) {
		returnedAt = time.Now()
		return ociannouncements.Page{}, nil
	})
	s := savedPages(func(context.Context, string) (ocistorage.NoticePage, error) {
		if time.Since(returnedAt) < 100*time.Millisecond {
			t.Fatal("list operations not paced")
		}
		return ocistorage.NoticePage{}, nil
	})
	if _, err := Enumerate(budget(t), a, s); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if err := pause(ctx, time.Hour); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("timer ignored deadline", err)
	}
}
