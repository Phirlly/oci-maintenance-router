package reconciliation

import (
	"context"
	"sync"
	"testing"

	"oci-maintenance-router/internal/notices"
)

func TestDiscoveryConcurrentCreateHasOneConfirmedWinner(t *testing.T) {
	n := candidateNotice(t, 1, nil, true)
	d, s, lease := discoveryFixture(t, []notices.Notice{n}, nil)
	a := d.collector.announcements.(announcementDetails)
	a.get = func(context.Context, string) (notices.Notice, error) { return n, nil }
	d.collector.announcements = a
	var ready sync.WaitGroup
	ready.Add(2)
	s.before = func(ctx context.Context, call discoveryCall) error {
		if call.method == "CREATE" {
			ready.Done()
			ready.Wait()
		}
		return ctx.Err()
	}
	type outcome struct {
		result DiscoveryResult
		err    error
	}
	results := make(chan outcome, 2)
	for range 2 {
		go func() { r, e := d.Run(leaseBudget(t), lease); results <- outcome{r, e} }()
	}
	success, conflicts := 0, 0
	for range 2 {
		out := <-results
		if out.err == nil {
			success++
			components := out.result.Candidates().Components()
			components[0].Members[0].ChainID = "caller mutation"
			if out.result.Candidates().Components()[0].Members[0].ChainID != "" {
				t.Fatal("result aliases caller snapshot")
			}
		} else {
			conflicts++
			discoveryFailure(t, out.result, out.err, "conflict")
		}
	}
	if success != 1 || conflicts != 1 || len(s.noticeWrites()) != 2 {
		t.Fatal("concurrent create retried or both workers claimed success")
	}
}

func TestDiscoveryResumesConfirmedPartialWritesWithFreshCollection(t *testing.T) {
	a, b := candidateNotice(t, 1, nil, true), candidateNotice(t, 2, nil, true)
	d, s, lease := discoveryFixture(t, []notices.Notice{a, b}, nil)
	s.before = func(_ context.Context, call discoveryCall) error {
		if call.method == "CREATE" && call.name == noticeName(b.Key().NoticeID) {
			return context.Canceled
		}
		return nil
	}
	got, err := d.Run(leaseBudget(t), lease)
	discoveryFailure(t, got, err, "confirmation_read")
	s.before = nil
	source := d.collector.announcements.(announcementDetails)
	source.get = func(_ context.Context, id string) (notices.Notice, error) {
		if id == a.Key().NoticeID {
			return a, nil
		}
		return b, nil
	}
	d.collector.announcements = source
	got, err = d.Run(leaseBudget(t), lease)
	if err != nil {
		t.Fatal("fresh complete pass did not resume", err)
	}
	if len(got.Candidates().Components()) != 2 {
		t.Fatal("partial work lost on restart")
	}
	for _, id := range []string{a.Key().NoticeID, b.Key().NoticeID} {
		if len(s.record(t, got.EnvironmentID(), id).Snapshot().Revisions) != 1 {
			t.Fatal("restart lost history")
		}
	}
}
