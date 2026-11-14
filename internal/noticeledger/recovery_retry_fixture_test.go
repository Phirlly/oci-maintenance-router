package noticeledger

import (
	"strings"
	"testing"
	"time"

	"oci-maintenance-router/internal/delivery"
)

func retryReady(t *testing.T, prior bool) Record {
	t.Helper()
	r := prepare(t, observed(t, source(db), db), preparations(t, source(db), db)...)
	if prior {
		s, ref := begin(t, r, r.Snapshot().Batches[0].Message.BatchID, clock())
		var err error
		r, err = s.Finish(ref, 0, clock(), clock())
		if err != nil {
			t.Fatal(err)
		}
	}
	r, err := r.Hold(delivery.ReasonOperator, clock())
	if err != nil {
		t.Fatal(err)
	}
	return recoveryRecord(t, r)
}

func retryRequest(t *testing.T, r Record) RecoveryRequest {
	t.Helper()
	b := r.Snapshot().Batches[0]
	gen := r.data.Revisions[revisionIndex(r.data, b.Message.Revision.Digest)].RoutingGeneration
	q := RecoveryRequest{RequestID: "retry-1", Kind: RetryBatch, TargetID: b.Message.BatchID, RoutingGeneration: gen, ComponentFingerprint: strings.Repeat("b", 64)}
	h, err := r.ReviewFingerprint(q.Kind, q.TargetID, q.RoutingGeneration, q.ComponentFingerprint)
	if err != nil {
		t.Fatal(err)
	}
	q.ReviewFingerprint = h
	return q
}

func retryContext(r Record, at time.Time) delivery.AttemptContext {
	return delivery.AttemptContext{OwnerToken: "manual-owner", RoutingGeneration: r.Snapshot().Batches[0].Message.RoutingGeneration, Now: at, LeaseExpiresAt: at.Add(time.Minute)}
}

func claimedRetry(t *testing.T, prior bool) (Record, RecoveryRequest, delivery.AttemptRef, time.Time) {
	t.Helper()
	r := retryReady(t, prior)
	q := retryRequest(t, r)
	at := clock().Add(48 * time.Hour)
	next, action, ref, err := r.RetryRecovery(q, retryContext(r, at))
	if err != nil || action.Status != RecoveryConsumed || ref.BatchID == "" {
		t.Fatal("retry not consumed", err)
	}
	return next, q, ref, at
}
