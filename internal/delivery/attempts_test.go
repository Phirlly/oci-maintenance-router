package delivery_test

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"oci-maintenance-router/internal/delivery"
)

func zeroTime() time.Time { return time.Time{} }

func TestAttemptOwnershipAndLateResults(t *testing.T) {
	r, ref := sending(t)
	b := r.Batches()[0]
	if b.State != delivery.Sending || b.AttemptCount != 1 || b.OwnerToken != ref.OwnerToken || !b.FirstAttemptAt.Equal(instant()) || !b.UpdatedAt.Equal(instant()) || b.Ambiguous {
		t.Fatal("attempt not recorded")
	}
	for _, stale := range []delivery.AttemptRef{{}, {BatchID: ref.BatchID, OwnerToken: "other", AttemptCount: 1}, {BatchID: ref.BatchID, OwnerToken: ref.OwnerToken, AttemptCount: 2}} {
		got, err := r.Finish(stale, 202, instant(), zeroTime())
		rejectedRecord(t, got, err)
	}
	for _, when := range []time.Time{instant().Add(-time.Millisecond), instant().Add(time.Minute), zeroTime()} {
		got, err := r.Finish(ref, 202, when, zeroTime())
		rejectedRecord(t, got, err)
	}
	recovered, err := r.Recover(instant().Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	next, nextRef, err := attempt(recovered, instant().Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if nextRef.AttemptCount != 2 || !next.Batches()[0].FirstAttemptAt.Equal(instant()) {
		t.Fatal("retry identity reset")
	}
	got, err := next.Finish(ref, 202, instant().Add(time.Minute), zeroTime())
	rejectedRecord(t, got, err)
	accepted, err := next.Finish(nextRef, 202, instant().Add(time.Minute), zeroTime())
	if err != nil {
		t.Fatal(err)
	}
	a := accepted.Batches()[0]
	if a.State != delivery.APIAccepted || !a.Ambiguous || !a.AcceptedAt.Equal(a.UpdatedAt) || !reflect.DeepEqual(a.Message, b.Message) || a.SecretVersion != 7 {
		t.Fatal("acceptance lost history")
	}
	got, _, err = attempt(accepted, instant().Add(2*time.Minute))
	rejectedRecord(t, got, err)
	got, err = accepted.Finish(nextRef, 202, instant().Add(time.Minute), zeroTime())
	rejectedRecord(t, got, err)
}

func TestBeginRejectsInvalidAuthorityAndTime(t *testing.T) {
	r, _ := prepared(t, db)
	b := r.Batches()[0]
	base := delivery.AttemptContext{OwnerToken: "owner", RoutingGeneration: b.Message.RoutingGeneration, Now: instant(), LeaseExpiresAt: instant().Add(600 * time.Second)}
	for _, change := range []func(*delivery.AttemptContext){
		func(c *delivery.AttemptContext) { c.OwnerToken = "" }, func(c *delivery.AttemptContext) { c.OwnerToken = "private/value" }, func(c *delivery.AttemptContext) { c.OwnerToken = strings.Repeat("a", 129) },
		func(c *delivery.AttemptContext) { c.RoutingGeneration = "changed" },
		func(c *delivery.AttemptContext) { c.Now = zeroTime() },
		func(c *delivery.AttemptContext) { c.LeaseExpiresAt = zeroTime() },
		func(c *delivery.AttemptContext) { c.Now = time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC) },
		func(c *delivery.AttemptContext) { c.LeaseExpiresAt = c.Now }, func(c *delivery.AttemptContext) { c.LeaseExpiresAt = c.Now.Add(600*time.Second + time.Millisecond) },
	} {
		c := base
		change(&c)
		got, ref, err := r.Begin(b.Message.BatchID, c)
		rejectedRecord(t, got, err)
		if ref != (delivery.AttemptRef{}) {
			t.Fatal("failed begin produced reference")
		}
	}
	got, _, err := r.Begin("missing", base)
	rejectedRecord(t, got, err)
	got, _, err = (delivery.Record{}).Begin(b.Message.BatchID, base)
	rejectedRecord(t, got, err)
	base.Now = instant().In(time.FixedZone("offset", 3600)).Add(123456789 * time.Nanosecond)
	base.LeaseExpiresAt = base.Now.Add(time.Minute)
	got, _, err = r.Begin(b.Message.BatchID, base)
	if err != nil {
		t.Fatal(err)
	}
	if got.Batches()[0].LastAttemptAt.Format(time.RFC3339Nano) != "2026-01-01T00:00:00.123Z" {
		t.Fatal("clock not normalized")
	}
}

func TestOnlyDueRetriesBeginAndHorizonIsInclusive(t *testing.T) {
	r, ref := sending(t)
	wait, err := r.Finish(ref, 429, instant(), instant().Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	got, _, err := attempt(wait, instant())
	rejectedRecord(t, got, err)
	for i := 1; i <= 4; i++ {
		r, ref, err = attempt(wait, instant().Add(time.Duration(i)*time.Second))
		if err != nil {
			t.Fatal(err)
		}
		wait, err = r.Finish(ref, 503, instant().Add(time.Duration(i)*time.Second), instant().Add(time.Duration(i+1)*time.Second))
		if err != nil {
			t.Fatal(err)
		}
	}
	if wait.Batches()[0].AttemptCount != 5 {
		t.Fatal("operation attempt budget became lifetime limit")
	}
	got, _, err = attempt(wait, instant().Add(24*time.Hour))
	rejectedRecord(t, got, err)
	last, ref, err := attempt(wait, instant().Add(24*time.Hour-time.Second))
	if err != nil {
		t.Fatal(err)
	}
	accepted, err := last.Finish(ref, 202, instant().Add(24*time.Hour), zeroTime())
	if err != nil || accepted.Batches()[0].State != delivery.APIAccepted {
		t.Fatalf("cutoff discarded acceptance: %v", err)
	}
}
