package reconciliation

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"
)

func TestOperatorChallengeStorePersistsExactFirstClaimAndOutcome(t *testing.T) {
	s, objects, _ := operatorStoreFixture(t)
	old := operatorSaved(t, s)
	if len(objects.calls) != 3 || objects.calls[0].method != "GET" || objects.calls[1].method != "HISTORY" || objects.calls[2].method != "CREATE" {
		t.Fatal("incorrect initialization sequence")
	}
	body, _ := operatorChallengeFixture(t).Encode()
	for _, call := range objects.calls {
		if call.name != operatorObjectName {
			t.Fatal("caller or unrelated path used", call.name)
		}
	}
	if !bytes.Equal(objects.calls[2].body, body) {
		t.Fatal("saved different claim bytes")
	}
	loaded, err := s.Load(leaseBudget(t), "operator-test-1")
	if err != nil {
		t.Fatal(err)
	}
	actual, _ := loaded.Record().Encode()
	if !bytes.Equal(actual, body) {
		t.Fatal("load changed claim")
	}
	next := operatorOutcome(t, old, 200)
	saved, err := s.Save(leaseBudget(t), old, next)
	if err != nil || saved.Record().Snapshot().State != OperatorAccepted {
		t.Fatal("outcome not saved", err)
	}
	if objects.calls[len(objects.calls)-1].tag != "v1" {
		t.Fatal("save not bound to observed ETag")
	}
	loaded, err = s.Load(leaseBudget(t), "operator-test-1")
	if err != nil || loaded.Record().Snapshot().State != OperatorAccepted {
		t.Fatal("terminal receipt lost on restart", err)
	}
	clear(body)
	snapshot := loaded.Record().Snapshot()
	clear(snapshot.Payload)
	if saved.Record().Snapshot().MessageID != operatorMessageID || !bytes.Contains(saved.Record().Snapshot().Payload, []byte("operator notification")) {
		t.Fatal("caller changed stored immutable identity")
	}
}

func TestOperatorChallengeCreateKeepsFirstClaimAndTerminalProgress(t *testing.T) {
	s, objects, _ := operatorStoreFixture(t)
	old := operatorSaved(t, s)
	c := operatorClaimFixture()
	c.ClaimToken = strings.Repeat("f", 64)
	c.Now = c.Now.Add(time.Second)
	c.ExpiresAt = c.ExpiresAt.Add(time.Second)
	competing, err := NewOperatorChallenge(operatorIntentFixture(t), c)
	if err != nil {
		t.Fatal(err)
	}
	for _, terminal := range []bool{false, true} {
		if terminal {
			old, err = s.Save(leaseBudget(t), old, operatorOutcome(t, old, 200))
			if err != nil {
				t.Fatal(err)
			}
		}
		before := objects.version
		calls := len(objects.calls)
		got, err := s.Create(leaseBudget(t), competing)
		actual, _ := got.Record().Encode()
		want, _ := old.Record().Encode()
		if err != nil || !bytes.Equal(actual, want) || objects.version != before || len(objects.calls) != calls+1 {
			t.Fatal("repeated intent reset original claim/pin/outcome", err)
		}
	}
}

func TestOperatorChallengeCreateRejectsChangedIntentAndInvalidInitialState(t *testing.T) {
	s, objects, _ := operatorStoreFixture(t)
	old := operatorSaved(t, s)
	i := operatorIntentFixture(t)
	i.SubscriptionID = "ocid1.onssubscription.oc1.iad.other"
	different, err := NewOperatorChallenge(i, operatorClaimFixture())
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.Create(leaseBudget(t), different)
	operatorStoredFailure(t, got, err, "identity_conflict", 200, false)
	before := len(objects.calls)
	for _, record := range []OperatorChallenge{OperatorChallenge{}, operatorOutcome(t, old, 200)} {
		got, err = s.Create(leaseBudget(t), record)
		operatorStoredFailure(t, got, err, "invalid_request", 0, false)
	}
	if len(objects.calls) != before || objects.version != 1 {
		t.Fatal("invalid initial proposal dispatched")
	}
}

func TestOperatorChallengeStoreLoadDoesNotInitialize(t *testing.T) {
	s, objects, _ := operatorStoreFixture(t)
	got, err := s.Load(leaseBudget(t), "operator-test-1")
	operatorStoredFailure(t, got, err, "read_failed", 404, false)
	if len(objects.calls) != 1 || objects.calls[0].method != "GET" || objects.version != 0 {
		t.Fatal("load attempted initialization")
	}
}

func TestOperatorChallengeStoreRequiresLiveDeadline(t *testing.T) {
	s, objects, _ := operatorStoreFixture(t)
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	for _, tc := range []struct {
		ctx  context.Context
		code string
	}{{nil, "invalid_request"}, {context.Background(), "deadline_required"}, {canceled, "canceled"}} {
		got, err := s.Create(tc.ctx, operatorChallengeFixture(t))
		operatorStoredFailure(t, got, err, tc.code, 0, false)
		got, err = s.Load(tc.ctx, "operator-test-1")
		operatorStoredFailure(t, got, err, tc.code, 0, false)
	}
	if len(objects.calls) != 0 {
		t.Fatal("invalid lifetime dispatched")
	}
}
