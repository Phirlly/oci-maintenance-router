package reconciliation

import (
	"context"
	"errors"
	"oci-maintenance-router/internal/ocistorage"
	"testing"
	"time"
)

func TestOperatorChallengeStorageSharesOneDeadlineAcrossAllStages(t *testing.T) {
	for _, operation := range []string{"load", "create-confirm", "save-confirm"} {
		for _, duration := range []time.Duration{time.Second, time.Hour} {
			s, objects, b := operatorStoreFixture(t)
			var old StoredOperatorChallenge
			if operation != "create-confirm" {
				old = operatorSaved(t, s)
			}
			var contexts []context.Context
			b.get = func(c context.Context, n string) (ocistorage.Object, error) {
				contexts = append(contexts, c)
				return objects.Get(c, n)
			}
			b.history = func(c context.Context, _ string) (bool, error) { contexts = append(contexts, c); return true, nil }
			b.create = func(c context.Context, n string, body []byte) (string, error) {
				contexts = append(contexts, c)
				_, err := objects.Create(c, n, body)
				return "", err
			}
			b.replace = func(c context.Context, n, tag string, body []byte) (string, error) {
				contexts = append(contexts, c)
				_, err := objects.Replace(c, n, tag, body)
				return "", err
			}
			ctx, cancel := context.WithTimeout(context.Background(), duration)
			t.Cleanup(cancel)
			before := time.Now()
			var err error
			switch operation {
			case "load":
				_, err = s.Load(ctx, "operator-test-1")
			case "create-confirm":
				_, err = s.Create(ctx, operatorChallengeFixture(t))
			case "save-confirm":
				_, err = s.Save(ctx, old, operatorOutcome(t, old, 200))
			}
			if err != nil || len(contexts) == 0 {
				t.Fatal("bounded operation failed", err)
			}
			for _, child := range contexts {
				deadline, ok := child.Deadline()
				parent, _ := ctx.Deadline()
				if child != contexts[0] || child.Err() != context.Canceled || !ok || (duration == time.Second && !deadline.Equal(parent)) || (duration == time.Hour && (deadline.Before(before.Add(10*time.Second)) || deadline.After(time.Now().Add(10*time.Second)))) {
					t.Fatal("stage renewed deadline or retained live child")
				}
			}
			expected := 1
			if operation == "create-confirm" {
				expected = 4
			} else if operation == "save-confirm" {
				expected = 2
			}
			if len(contexts) != expected {
				t.Fatal("missing bounded stage")
			}
		}
	}
}

func TestOperatorChallengeSaveRejectsExpiredOrMissingLifetime(t *testing.T) {
	s, objects, _ := operatorStoreFixture(t)
	old := operatorSaved(t, s)
	next := operatorOutcome(t, old, 200)
	canceled, cancel := context.WithCancel(leaseBudget(t))
	cancel()
	expired, end := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer end()
	before := len(objects.calls)
	for _, tc := range []struct {
		ctx  context.Context
		code string
	}{{nil, "invalid_request"}, {context.Background(), "deadline_required"}, {canceled, "canceled"}, {expired, "deadline_exceeded"}} {
		got, err := s.Save(tc.ctx, old, next)
		operatorStoredFailure(t, got, err, tc.code, 0, false)
		if tc.ctx != nil && tc.ctx.Err() != nil && !errors.Is(err, tc.ctx.Err()) {
			t.Fatal("lost cancellation identity")
		}
	}
	if len(objects.calls) != before {
		t.Fatal("invalid lifetime dispatched")
	}
}
