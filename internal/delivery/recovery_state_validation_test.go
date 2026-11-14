package delivery

import (
	"testing"
	"time"
)

func recoveryStateFixture(t *testing.T) (Record, Record, AttemptRef, time.Time) {
	t.Helper()
	r, _ := codecFixture(t, "recovery")
	r, err := r.MigrateRecovery()
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	id := r.Batches()[0].Message.BatchID
	held, err := r.Hold(id, ReasonOperator, at)
	if err != nil {
		t.Fatal(err)
	}
	s, ref, err := held.BeginManual(id, AttemptContext{OwnerToken: "manual", RoutingGeneration: held.Batches()[0].Message.RoutingGeneration, Now: at, LeaseExpiresAt: at.Add(time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	return r, s, ref, at
}

func TestRecoveryDecoderRejectsInconsistentManualProvenance(t *testing.T) {
	p, s, ref, at := recoveryStateFixture(t)
	accepted, err := s.Finish(ref, 202, at, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	transient, err := s.Finish(ref, 503, at, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	network, err := s.Finish(ref, 0, at, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	expired, err := s.Recover(at.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		record Record
		change func(*savedBatch)
	}{
		{"attempted missing kind", s, func(b *savedBatch) { b.AttemptKind = nil }},
		{"attempted empty kind", s, func(b *savedBatch) { kind := AttemptKind(""); b.AttemptKind = &kind }},
		{"attempted unknown kind", s, func(b *savedBatch) { kind := AttemptKind("OTHER"); b.AttemptKind = &kind }},
		{"prepared manual", p, func(b *savedBatch) { kind := ManualAttempt; b.AttemptKind = &kind }},
		{"manual sending first ambiguous", s, func(b *savedBatch) { b.Ambiguous = true }},
		{"manual accepted first ambiguous", accepted, func(b *savedBatch) { b.Ambiguous = true }},
		{"manual transient first ambiguous", transient, func(b *savedBatch) { b.Ambiguous = true }},
		{"manual network not ambiguous", network, func(b *savedBatch) { b.Ambiguous = false }},
		{"manual expiry not ambiguous", expired, func(b *savedBatch) { b.Ambiguous = false }},
		{"manual network after expiry", network, func(b *savedBatch) { b.UpdatedAt = b.AttemptExpiresAt }},
		{"manual expiry too early", expired, func(b *savedBatch) { b.UpdatedAt = b.LastAttemptAt }},
		{"manual transient after expiry", transient, func(b *savedBatch) { b.UpdatedAt = b.AttemptExpiresAt }},
		{"manual horizon reason", transient, func(b *savedBatch) { b.Reason = ReasonHorizon }},
		{"manual source hold", transient, func(b *savedBatch) { b.Reason = ReasonSourceChanged }},
		{"manual automatic continuation", transient, func(b *savedBatch) { b.State = RetryWait; b.NextAttemptAt = b.UpdatedAt }},
		{"manual next time", transient, func(b *savedBatch) { b.NextAttemptAt = b.UpdatedAt }},
		{"automatic horizon bypass", s, func(b *savedBatch) {
			kind := AutomaticAttempt
			b.AttemptKind = &kind
			b.AttemptCount = 2
			b.FirstAttemptAt = timeText(at.Add(-48 * time.Hour))
		}},
	} {
		t.Run(tc.name, func(t *testing.T) { d := tc.record.copy(); tc.change(&d.Batches[0]); rejectedData(t, d) })
	}
	for _, r := range []Record{p, s, accepted, transient, network, expired} {
		d := r.copy()
		d.SchemaVersion = 1
		rejectedData(t, d)
	}
}

func TestRecoveryDecoderRejectsClosedStateRewritingHistory(t *testing.T) {
	p, s, ref, at := recoveryStateFixture(t)
	held, err := s.StopUnsent(ref, at)
	if err != nil {
		t.Fatal(err)
	}
	for _, base := range []Record{p, held} {
		closed, err := base.Close(base.Batches()[0].Message.BatchID, at)
		if err != nil {
			t.Fatal(err)
		}
		for _, change := range []func(*savedBatch){
			func(b *savedBatch) { b.Reason = "" },
			func(b *savedBatch) { b.Reason = ReasonExpired },
			func(b *savedBatch) { b.UpdatedAt = nil },
			func(b *savedBatch) { b.AcceptedAt = timeText(at) },
			func(b *savedBatch) { b.NextAttemptAt = timeText(at) },
		} {
			d := closed.copy()
			change(&d.Batches[0])
			rejectedData(t, d)
		}
		d := closed.copy()
		d.SchemaVersion = 1
		d.Batches[0].AttemptKind = nil
		rejectedData(t, d)
	}
}
