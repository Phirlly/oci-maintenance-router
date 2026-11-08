package delivery_test

import (
	"testing"
	"time"

	"oci-maintenance-router/internal/delivery"
)

func TestHTTPAndNetworkOutcomes(t *testing.T) {
	for _, status := range []int{0, 100, 200, 201, 202, 204, 301, 400, 401, 404, 408, 429, 499, 500, 503, 599} {
		r, ref := sending(t)
		retry := zeroTime()
		want := delivery.ReviewRequired
		reason := delivery.ReasonPermanent
		if status == 202 {
			want = delivery.APIAccepted
			reason = ""
		}
		if status == 0 || status == 429 || status >= 500 {
			retry = instant().Add(time.Second)
			want = delivery.RetryWait
			reason = delivery.ReasonTransient
		}
		if status == 0 {
			reason = delivery.ReasonNetwork
		}
		after, err := r.Finish(ref, status, instant(), retry)
		if err != nil {
			t.Fatal(err)
		}
		b := after.Batches()[0]
		if b.State != want || b.Reason != reason || b.Ambiguous != (status == 0) || b.AttemptCount != 1 || r.Batches()[0].State != delivery.Sending {
			t.Fatalf("status %d wrong outcome: %+v", status, b)
		}
	}
}

func TestFinishRejectsInvalidResponseArguments(t *testing.T) {
	r, ref := sending(t)
	for _, tc := range []struct {
		status int
		retry  time.Time
	}{{-1, zeroTime()}, {99, zeroTime()}, {600, zeroTime()}, {202, instant()}, {400, instant()}, {429, zeroTime()}, {0, instant().Add(-time.Millisecond)}, {503, time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC)}} {
		got, err := r.Finish(ref, tc.status, instant(), tc.retry)
		rejectedRecord(t, got, err)
	}
	got, err := (delivery.Record{}).Finish(ref, 202, instant(), zeroTime())
	rejectedRecord(t, got, err)
}

func TestRetryAtHorizonRequiresReview(t *testing.T) {
	for _, status := range []int{0, 429, 503} {
		r, ref := sending(t)
		after, err := r.Finish(ref, status, instant(), instant().Add(24*time.Hour))
		if err != nil {
			t.Fatal(err)
		}
		b := after.Batches()[0]
		if b.State != delivery.ReviewRequired || b.Reason != delivery.ReasonHorizon || !b.NextAttemptAt.IsZero() || b.Ambiguous != (status == 0) {
			t.Fatal("retry beyond horizon remained eligible")
		}
	}
}
