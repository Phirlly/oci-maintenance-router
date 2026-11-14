package noticeledger

import (
	"strings"
	"testing"
	"time"

	"oci-maintenance-router/internal/delivery"
	"oci-maintenance-router/internal/notices"
	"oci-maintenance-router/internal/routing"
)

func approvalRequest(t *testing.T, r Record, target string) RecoveryRequest {
	t.Helper()
	q := RecoveryRequest{RequestID: "approve-1", Kind: ApproveRevision, TargetID: target,
		RoutingGeneration: r.data.Revisions[revisionIndex(r.data, target)].RoutingGeneration, ComponentFingerprint: strings.Repeat("b", 64)}
	var err error
	q.ReviewFingerprint, err = r.ReviewFingerprint(q.Kind, q.TargetID, q.RoutingGeneration, q.ComponentFingerprint)
	if err != nil {
		t.Fatal(err)
	}
	return q
}

func approvedRecord(t *testing.T, held bool) (Record, RecoveryRequest, RecoveryAction) {
	t.Helper()
	r := observed(t, source(db), db)
	var err error
	if held {
		r, err = r.Hold(delivery.ReasonOperator, clock())
		if err != nil {
			t.Fatal(err)
		}
	}
	r = recoveryRecord(t, r)
	q := approvalRequest(t, r, r.Snapshot().Revisions[0].Digest)
	next, a, err := observer(t, nil).ApproveRecovery(r, q, normalized(t, source(db)), false, lookups(db), clock(), preparations(t, source(db), db)...)
	if err != nil || a.Status != RecoveryApproved {
		t.Fatal("approval failed", a, err)
	}
	return next, q, a
}

func approvedContext(r Record, id string, at time.Time) delivery.AttemptContext {
	b, _ := r.data.delivery.Batch(id)
	return delivery.AttemptContext{OwnerToken: "approved-owner", RoutingGeneration: b.Message.RoutingGeneration, Now: at, LeaseExpiresAt: at.Add(time.Minute)}
}

func approvalDrafts(t *testing.T, n notices.Notice, found map[string]routing.Lookup, ids ...string) []delivery.Preparation {
	t.Helper()
	b, err := delivery.New(settings(nil), catalog(), location())
	if err != nil {
		t.Fatal(err)
	}
	result, err := b.Build(n, false, found, ids)
	if err != nil || len(result.Issues) != 0 {
		t.Fatal(err, result.Issues)
	}
	values := []delivery.Preparation{}
	for _, draft := range result.Drafts {
		values = append(values, delivery.Preparation{Draft: draft, SecretVersion: 7})
	}
	return values
}
