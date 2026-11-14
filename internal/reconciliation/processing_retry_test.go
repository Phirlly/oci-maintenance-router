package reconciliation

import (
	"context"
	"oci-maintenance-router/internal/delivery"
	"oci-maintenance-router/internal/pagerduty"
	"testing"
	"time"
)

func TestMaintenanceRetriesThreeTimesPerRunWithPinnedIdentity(t *testing.T) {
	n := processingNotice(t, 1, processingDB)
	f := processingFixtureFor(t, n)
	send := f.p.senders["US"]
	f.p.senders["US"] = func(ctx context.Context, body, key []byte) (pagerduty.Response, error) {
		response, err := send(ctx, body, key)
		response.Status = 503
		return response, err
	}
	s := f.run(t)
	if s.Accepted != 0 || s.Deferred != 1 || len(s.Attempts) != 3 || f.exact.Load() != 3 || f.current.Load() != 1 {
		t.Fatalf("wrong retry budget: %+v", s)
	}
	b := f.saved(t, n.Key().NoticeID).Snapshot().Batches[0]
	if b.State != delivery.RetryWait || b.AttemptCount != 3 || b.Ambiguous {
		t.Fatal("wrong transient state")
	}
	f.p.senders["US"] = send
	again := f.run(t)
	a := f.saved(t, n.Key().NoticeID).Snapshot().Batches[0]
	if again.Accepted != 1 || a.AttemptCount != 4 || !a.FirstAttemptAt.Equal(b.FirstAttemptAt) || a.Message.PayloadDigest != b.Message.PayloadDigest || a.SecretVersion != b.SecretVersion || f.current.Load() != 1 {
		t.Fatal("retry reset identity or used CURRENT")
	}
}

func TestMaintenanceDefers429AndHonorsRetryAfter(t *testing.T) {
	for _, explicit := range []bool{false, true} {
		n := processingNotice(t, 1, processingDB)
		f := processingFixtureFor(t, n)
		send := f.p.senders["US"]
		var hint time.Time
		f.p.senders["US"] = func(ctx context.Context, body, key []byte) (pagerduty.Response, error) {
			_, err := send(ctx, body, key)
			if explicit {
				hint = time.Now().Add(10 * time.Minute).UTC().Truncate(time.Millisecond)
			}
			return pagerduty.Response{Status: 429, RetryAfter: hint}, err
		}
		f.p.jitter = func(limit time.Duration) time.Duration { return limit }
		s := f.run(t)
		b := f.saved(t, n.Key().NoticeID).Snapshot().Batches[0]
		if len(s.Attempts) != 1 || s.Deferred != 1 || b.State != delivery.RetryWait || b.NextAttemptAt.Sub(b.UpdatedAt) < 210*time.Second {
			t.Fatal("429 was retried early")
		}
		if explicit && !b.NextAttemptAt.Equal(hint) {
			t.Fatal("Retry-After shortened")
		}
		f.run(t)
		if f.sent.Load() != 1 {
			t.Fatal("future retry dispatched on restart")
		}
	}
}

func TestMaintenanceRetryOverflowBecomesDurableHorizonReview(t *testing.T) {
	n := processingNotice(t, 1, processingDB)
	f := processingFixtureFor(t, n)
	send := f.p.senders["US"]
	f.p.senders["US"] = func(ctx context.Context, body, key []byte) (pagerduty.Response, error) {
		_, err := send(ctx, body, key)
		return pagerduty.Response{Status: 503, RetryAfterOverflow: true}, err
	}
	s := f.run(t)
	b := f.saved(t, n.Key().NoticeID).Snapshot().Batches[0]
	if len(s.Attempts) != 1 || s.Attempts[0].Status != 503 || !s.Attempts[0].Confirmed || b.State != delivery.ReviewRequired || b.Reason != delivery.ReasonHorizon || !b.NextAttemptAt.IsZero() {
		t.Fatal("overflow lost actual response or review")
	}
}

func TestProcessingRetryPolicyBoundsLifetimeCountsAndJitter(t *testing.T) {
	at := time.Now().UTC().Truncate(time.Millisecond)
	r := &processingRun{owner: &MaintenanceProcessor{}}
	for _, test := range []struct {
		count          uint64
		status         int
		limit, minimum time.Duration
	}{{1, 503, time.Second, 0}, {2, 0, 2 * time.Second, 0}, {5, 500, 16 * time.Second, 0}, {6, 599, 30 * time.Second, 0}, {^uint64(0), 503, 30 * time.Second, 0}, {1, 429, 30 * time.Second, 180 * time.Second}} {
		r.owner.jitter = func(limit time.Duration) time.Duration {
			if limit != test.limit {
				t.Error("wrong jitter cap")
			}
			return limit
		}
		next, err := r.retryAt(delivery.BatchSnapshot{FirstAttemptAt: at, AttemptCount: test.count}, pagerduty.Response{}, test.status, at)
		if err != nil || !next.Equal(at.Add(test.minimum+test.limit)) {
			t.Fatal("wrong retry schedule", err)
		}
	}
	r.owner.jitter = func(time.Duration) time.Duration { return 0 }
	b := delivery.BatchSnapshot{FirstAttemptAt: at, AttemptCount: 1}
	next, err := r.retryAt(b, pagerduty.Response{RetryAfter: at.Add(48 * time.Hour)}, 503, at)
	if err != nil || !next.Equal(at.Add(24*time.Hour)) {
		t.Fatal("horizon not bounded")
	}
	for _, bad := range []time.Duration{-1, 31 * time.Second} {
		r.owner.jitter = func(time.Duration) time.Duration { return bad }
		if _, err = r.retryAt(b, pagerduty.Response{}, 503, at); err == nil {
			t.Fatal("invalid jitter admitted")
		}
	}
	for _, status := range []int{202, 400} {
		next, err = r.retryAt(b, pagerduty.Response{}, status, at)
		if err != nil || !next.IsZero() {
			t.Fatal("permanent/accepted result scheduled retry")
		}
	}
}

func TestProcessingRetryAfterHorizonDoesNotProposePastOutcomeTime(t *testing.T) {
	first := time.Now().UTC().Truncate(time.Millisecond)
	at := first.Add(24*time.Hour + time.Millisecond)
	r := &processingRun{owner: &MaintenanceProcessor{jitter: func(time.Duration) time.Duration { return 0 }}}
	for _, response := range []pagerduty.Response{{Status: 503}, {Status: 429}, {Status: 503, RetryAfterOverflow: true}, {}} {
		next, err := r.retryAt(delivery.BatchSnapshot{FirstAttemptAt: first, AttemptCount: 2}, response, response.Status, at)
		if err != nil || next.Before(at) {
			t.Fatal("horizon decision cannot be persisted at observed time", err)
		}
	}
}
