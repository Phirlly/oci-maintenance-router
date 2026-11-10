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

func TestSavedSourceSharesOriginalDeadlineAndPacing(t *testing.T) {
	n := candidateNotice(t, 1, nil, true)
	r := candidateRecord(t, n)
	for _, duration := range []time.Duration{time.Minute, 5 * time.Minute} {
		ctx, cancel := context.WithTimeout(context.Background(), duration)
		var deadline time.Time
		var order []string
		check := func(ctx context.Context, step string) {
			d, ok := ctx.Deadline()
			if !ok {
				t.Fatal("missing deadline")
			}
			if deadline.IsZero() {
				deadline = d
			} else if !deadline.Equal(d) {
				t.Fatal("renewed deadline")
			}
			order = append(order, step)
		}
		a := announcementDetails{page: func(ctx context.Context, _ string) (ociannouncements.Page, error) {
			check(ctx, "list current")
			return ociannouncements.Page{}, nil
		}, get: func(ctx context.Context, _ string) (notices.Notice, error) {
			check(ctx, "get saved source")
			return n, nil
		}}
		s := savedDetails{page: func(ctx context.Context, _ string) (ocistorage.NoticePage, error) {
			check(ctx, "list saved")
			return ocistorage.NoticePage{Names: []string{recordName(r)}}, nil
		}, load: func(ctx context.Context, _ string) (noticeledger.Record, error) { check(ctx, "load"); return r, nil }}
		wait := func(ctx context.Context, d time.Duration) error {
			if d != 100*time.Millisecond {
				t.Fatal("wrong pacing")
			}
			check(ctx, "wait")
			return nil
		}
		before := time.Now()
		got, e := collector(t, a, s).collect(ctx, wait)
		cancel()
		if e != nil || len(got.Current) != 1 || !slices.Equal(order, []string{"list current", "wait", "list saved", "wait", "load", "wait", "get saved source"}) {
			t.Fatal("saved-source stage missing/unpaced", order, e)
		}
		if duration == time.Minute {
			parent, _ := ctx.Deadline()
			if !parent.Equal(deadline) {
				t.Fatal("extended caller")
			}
		} else if deadline.Before(before.Add(120*time.Second)) || deadline.After(time.Now().Add(120*time.Second)) {
			t.Fatal("wrong discovery budget")
		}
	}
}

func TestSavedSourceCancellationPrecedesUnavailableClassification(t *testing.T) {
	n := candidateNotice(t, 1, nil, true)
	r := candidateRecord(t, n)
	for _, phase := range []string{"wait", "read", "deadline"} {
		a, s := detailSources(t, nil, []noticeledger.Record{r})
		ctx, cancel := context.WithCancel(budget(t))
		if phase == "deadline" {
			cancel()
			ctx, cancel = context.WithTimeout(context.Background(), 20*time.Millisecond)
		}
		reads, waits := 0, 0
		a.get = func(ctx context.Context, _ string) (notices.Notice, error) {
			reads++
			if phase == "wait" {
				t.Fatal("dispatched after wait cancellation")
			}
			if phase == "deadline" {
				<-ctx.Done()
			} else {
				cancel()
			}
			return notices.Notice{}, &ociannouncements.Error{Code: ociannouncements.Service, Status: 404}
		}
		wait := func(context.Context, time.Duration) error {
			waits++
			if phase == "wait" && waits == 3 {
				cancel()
				return errors.New("private")
			}
			return nil
		}
		got, e := collector(t, a, s).collect(ctx, wait)
		cancel()
		code, status := Canceled, 404
		if phase == "wait" {
			status = 0
		}
		if phase == "deadline" {
			code = DeadlineExceeded
		}
		wantCollectionFailure(t, got, e, code, Announcements, status)
		if (phase == "wait" && reads != 0) || (phase != "wait" && reads != 1) {
			t.Fatal("retried or late dispatch")
		}
	}
}
