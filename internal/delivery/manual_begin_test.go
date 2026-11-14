package delivery_test

import (
	"bytes"
	"oci-maintenance-router/internal/delivery"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestManualBeginPreservesHistoryBeyondAutomaticHorizon(t *testing.T) {
	for _, prior := range []bool{false, true} {
		r := manualReady(t, prior)
		before := r.Batches()[0]
		old := encoded(t, r)
		at := instant().Add(48 * time.Hour)
		next, ref, err := r.BeginManual(before.Message.BatchID, manualContext(r, at))
		if err != nil {
			t.Fatal(err)
		}
		b := next.Batches()[0]
		first := before.FirstAttemptAt
		if !prior {
			first = at
		}
		if b.State != delivery.Sending || b.AttemptKind != delivery.ManualAttempt || b.AttemptCount != before.AttemptCount+1 || !b.FirstAttemptAt.Equal(first) || !b.LastAttemptAt.Equal(at) || b.Ambiguous != prior || b.OwnerToken != ref.OwnerToken || b.AttemptCount != ref.AttemptCount || ref.BatchID != b.Message.BatchID || b.SecretVersion != before.SecretVersion || !reflect.DeepEqual(b.Message, before.Message) {
			t.Fatal("manual attempt reset identity/history")
		}
		if !bytes.Equal(old, encoded(t, r)) {
			t.Fatal("predecessor mutated")
		}
		restored(t, next)
		got, _, err := attempt(r, at)
		rejectedRecord(t, got, err)
	}
}

func TestManualBeginKeepsAuthorityClockAndStateAdmission(t *testing.T) {
	r := manualReady(t, true)
	id := r.Batches()[0].Message.BatchID
	at := instant().Add(48 * time.Hour)
	base := manualContext(r, at)
	for _, change := range []func(*delivery.AttemptContext){
		func(c *delivery.AttemptContext) { c.OwnerToken = "" },
		func(c *delivery.AttemptContext) { c.OwnerToken = strings.Repeat("x", 129) },
		func(c *delivery.AttemptContext) { c.RoutingGeneration = "changed" },
		func(c *delivery.AttemptContext) { c.Now = time.Time{} },
		func(c *delivery.AttemptContext) { c.Now = instant().Add(-time.Millisecond) },
		func(c *delivery.AttemptContext) { c.LeaseExpiresAt = time.Time{} },
		func(c *delivery.AttemptContext) { c.LeaseExpiresAt = c.Now },
		func(c *delivery.AttemptContext) { c.LeaseExpiresAt = c.Now.Add(600*time.Second + time.Millisecond) },
	} {
		c := base
		change(&c)
		got, ref, err := r.BeginManual(id, c)
		rejectedRecord(t, got, err)
		if ref != (delivery.AttemptRef{}) {
			t.Fatal("invalid attempt returned reference")
		}
	}
	legacy, _ := prepared(t, db)
	legacy, err := legacy.Hold(id, delivery.ReasonOperator, instant())
	if err != nil {
		t.Fatal(err)
	}
	p, _ := prepared(t, db)
	s, ref := sending(t)
	accepted, err := s.Finish(ref, 202, instant(), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	wait, err := s.Finish(ref, 503, instant(), instant())
	if err != nil {
		t.Fatal(err)
	}
	for _, invalid := range []delivery.Record{{}, legacy, upgraded(t, p), upgraded(t, s), upgraded(t, accepted), upgraded(t, wait)} {
		got, ref, err := invalid.BeginManual(id, base)
		rejectedRecord(t, got, err)
		if ref != (delivery.AttemptRef{}) {
			t.Fatal("rejected state returned reference")
		}
	}
	got, _, err := r.BeginManual("missing", base)
	rejectedRecord(t, got, err)
}

func TestManualBeginCannotWrapCumulativeAttemptCount(t *testing.T) {
	r := manualReady(t, true)
	b := r.Batches()[0]
	body := bytes.Replace(encoded(t, r), []byte(`"attemptCount":1`), []byte(`"attemptCount":18446744073709551615`), 1)
	r, err := delivery.DecodeRecord(body, b.Message.EnvironmentID, b.Message.Revision.NoticeID)
	if err != nil {
		t.Fatal(err)
	}
	got, ref, err := r.BeginManual(b.Message.BatchID, manualContext(r, instant().Add(48*time.Hour)))
	rejectedRecord(t, got, err)
	if err.Error() != "attempt: count_exhausted" || ref != (delivery.AttemptRef{}) {
		t.Fatal("wrong overflow handling", err)
	}
}
