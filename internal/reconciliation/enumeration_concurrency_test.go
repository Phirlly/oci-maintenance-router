package reconciliation

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"oci-maintenance-router/internal/ociannouncements"
	"oci-maintenance-router/internal/ocistorage"
)

func TestConcurrentEnumerationsHaveIndependentCursorsAndResults(t *testing.T) {
	ctx := budget(t)
	a := announcementPages(func(_ context.Context, cursor string) (ociannouncements.Page, error) {
		switch cursor {
		case "":
			return ociannouncements.Page{IDs: []string{noticeID(1)}, NextPage: "second"}, nil
		case "second":
			return ociannouncements.Page{IDs: []string{noticeID(2)}}, nil
		default:
			t.Error("unexpected source cursor")
			return ociannouncements.Page{}, errors.New("invalid cursor")
		}
	})
	s := savedPages(func(_ context.Context, cursor string) (ocistorage.NoticePage, error) {
		if cursor != "" {
			t.Error("unexpected saved cursor")
		}
		return ocistorage.NoticePage{Names: []string{objectName(1)}}, nil
	})
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, err := enumerate(ctx, a, s, noPause)
			if err != nil || len(got.AnnouncementIDs) != 2 || got.AnnouncementIDs[0] != noticeID(1) || got.AnnouncementIDs[1] != noticeID(2) || len(got.NoticeObjects) != 1 || got.NoticeObjects[0] != objectName(1) {
				t.Error("concurrent result changed", err)
				return
			}
			got.AnnouncementIDs[0], got.NoticeObjects[0] = "changed", "changed"
		}()
	}
	wg.Wait()
}

func TestFailedEnumerationNeverResumesCursorOrRetainsBudget(t *testing.T) {
	fail := true
	var calls []string
	a := announcementPages(func(_ context.Context, cursor string) (ociannouncements.Page, error) {
		calls = append(calls, cursor)
		if cursor == "" {
			return ociannouncements.Page{IDs: []string{noticeID(1)}, NextPage: "second"}, nil
		}
		if fail {
			return ociannouncements.Page{}, errors.New("private")
		}
		return ociannouncements.Page{IDs: []string{noticeID(2)}}, nil
	})
	got, err := enumerate(budget(t), a, savedPages(emptySaved), noPause)
	wantFailure(t, got, err, ReadFailed, Announcements, 0)
	fail = false
	got, err = enumerate(budget(t), a, savedPages(emptySaved), noPause)
	if err != nil || len(calls) != 4 || calls[2] != "" || len(got.AnnouncementIDs) != 2 {
		t.Fatal("failure resumed partial work", err, calls)
	}
}

func TestWaitReturningSuccessAfterCancellationCannotDispatch(t *testing.T) {
	ctx, cancel := context.WithCancel(budget(t))
	defer cancel()
	a := announcementPages(func(_ context.Context, cursor string) (ociannouncements.Page, error) {
		if cursor != "" {
			t.Fatal("late page dispatch")
		}
		return ociannouncements.Page{NextPage: "next"}, nil
	})
	wait := func(context.Context, time.Duration) error { cancel(); return nil }
	got, err := enumerate(ctx, a, savedPages(emptySaved), wait)
	wantFailure(t, got, err, Canceled, Announcements, 0)
}
