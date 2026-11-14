package noticeledger

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"testing"
	"time"

	"oci-maintenance-router/internal/delivery"
)

func preparationFailure(t *testing.T, r Record, code string, at time.Time, indexes ...int) PreparationFailure {
	t.Helper()
	f, err := observer(t, nil).PreparationFailure(r, normalized(t, source(db, otherDB)), false, lookups(db, otherDB), indexes, code, at)
	if err != nil {
		t.Fatal("valid preparation evidence rejected", err)
	}
	return f
}

func preparationConfigDigest() string {
	hash := sha256.Sum256(settings(nil))
	return hex.EncodeToString(hash[:])
}

func TestPreparationIssuesRetainFirstAndLatestObservationAcrossRestart(t *testing.T) {
	r := observed(t, source(db, otherDB), db, otherDB)
	f := preparationFailure(t, r, "secret_unavailable", clock(), 0)
	p, err := (PreparationIssues{}).Observe(f)
	if err != nil {
		t.Fatal("first observation rejected", err)
	}
	body, err := p.Encode()
	if err != nil {
		t.Fatal(err)
	}
	for _, private := range []string{noticeID, db, "vaultsecret", "owner"} {
		if bytes.Contains(body, []byte(private)) {
			t.Fatal("copied unrelated input", private)
		}
	}
	p, err = DecodePreparationIssues(body, r)
	if err != nil {
		t.Fatal("restart rejected", err)
	}
	next, err := p.Observe(preparationFailure(t, r, "state_capacity", clock().Add(time.Second), 0, 1))
	if err != nil {
		t.Fatal(err)
	}
	s := next.Snapshot()
	if len(s.Observations) != 2 {
		t.Fatal("lost subjects")
	}
	a, b := s.Observations[0], s.Observations[1]
	if a.FirstSeenAt != clock().Format(timeFormat) || a.LastSeenAt != clock().Add(time.Second).Format(timeFormat) || b.FirstSeenAt != b.LastSeenAt || a.Code != "state_capacity" {
		t.Fatal("wrong first/latest semantics", s)
	}
	if a.ContextDigest != p.Snapshot().Observations[0].ContextDigest || a.DestinationID != "payments" || a.RoutingGeneration != r.Snapshot().Revisions[0].RoutingGeneration {
		t.Fatal("context not bound")
	}
	s.Observations[0].Code = "changed"
	if next.Snapshot().Observations[0].Code != "state_capacity" || p.Snapshot().Observations[0].Code != "secret_unavailable" {
		t.Fatal("observation aliases prior/caller data")
	}
	again, err := next.Observe(preparationFailure(t, r, "state_capacity", clock().Add(time.Second), 0, 1))
	x, _ := next.Encode()
	y, _ := again.Encode()
	if err != nil || !bytes.Equal(x, y) {
		t.Fatal("same evidence changed bytes", err)
	}
}

func TestPreparationFailureCorrelatesHeldRowsWithoutClearingHold(t *testing.T) {
	r := observed(t, source(db, otherDB), db, otherDB)
	r, err := r.Hold(delivery.ReasonOperator, clock())
	if err != nil {
		t.Fatal(err)
	}
	before := encoded(t, r)
	f := preparationFailure(t, r, "payload_too_large", clock(), 1)
	p, err := (PreparationIssues{}).Observe(f)
	if err != nil || p.Snapshot().Observations[0].DestinationID != "payments" {
		t.Fatal("held evidence lost current destination", err)
	}
	if !bytes.Equal(before, encoded(t, f.Notice())) || r.Snapshot().Revisions[0].Resources[1].DestinationID != "" {
		t.Fatal("evidence cleared hold or rewrote saved row")
	}
}

func TestPreparationFailureRejectsInvalidTargetsAndTimes(t *testing.T) {
	r := observed(t, source(db, otherDB), db, otherDB)
	for _, indexes := range [][]int{nil, {}, {-1}, {2}, {0, 0}} {
		f, err := observer(t, nil).PreparationFailure(r, normalized(t, source(db, otherDB)), false, lookups(db, otherDB), indexes, "secret_invalid", clock())
		if err == nil || f.Notice().data != nil {
			t.Fatal("invalid target accepted", indexes)
		}
	}
	for _, at := range []time.Time{{}, clock().Add(-time.Second)} {
		f, err := observer(t, nil).PreparationFailure(r, normalized(t, source(db, otherDB)), false, lookups(db, otherDB), []int{0}, "secret_invalid", at)
		if err == nil || f.Notice().data != nil {
			t.Fatal("invalid time accepted")
		}
	}
	if _, err := (PreparationIssues{}).Observe(PreparationFailure{}); err == nil {
		t.Fatal("empty evidence accepted")
	}
}
