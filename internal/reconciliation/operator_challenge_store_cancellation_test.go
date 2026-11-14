package reconciliation

import (
	"context"
	"errors"
	"oci-maintenance-router/internal/ocistorage"
	"testing"
)

func TestOperatorChallengeStorageRejectsLateSuccessAtEveryStage(t *testing.T) {
	for _, stage := range []string{"load", "initial-read", "history", "create", "create-confirm", "save", "save-confirm"} {
		t.Run(stage, func(t *testing.T) {
			s, objects, b := operatorStoreFixture(t)
			var old StoredOperatorChallenge
			if stage == "load" || stage == "save" || stage == "save-confirm" {
				old = operatorSaved(t, s)
			}
			ctx, cancel := context.WithCancel(leaseBudget(t))
			defer cancel()
			writes := 0
			b.get = func(c context.Context, n string) (ocistorage.Object, error) {
				obj, err := objects.Get(c, n)
				if stage == "load" || stage == "initial-read" || ((stage == "create-confirm" || stage == "save-confirm") && writes > 0) {
					cancel()
				}
				return obj, err
			}
			b.history = func(context.Context, string) (bool, error) {
				if stage == "history" {
					cancel()
				}
				return true, nil
			}
			b.create = func(c context.Context, n string, body []byte) (string, error) {
				writes++
				tag, err := objects.Create(c, n, body)
				if stage == "create" {
					cancel()
				}
				if stage == "create-confirm" {
					tag = ""
				}
				return tag, err
			}
			b.replace = func(c context.Context, n, tag string, body []byte) (string, error) {
				writes++
				saved, err := objects.Replace(c, n, tag, body)
				if stage == "save" {
					cancel()
				}
				if stage == "save-confirm" {
					saved = ""
				}
				return saved, err
			}
			var got StoredOperatorChallenge
			var err error
			switch stage {
			case "load":
				got, err = s.Load(ctx, "operator-test-1")
			case "save", "save-confirm":
				got, err = s.Save(ctx, old, operatorOutcome(t, old, 200))
			default:
				got, err = s.Create(ctx, operatorChallengeFixture(t))
			}
			var detail *OperatorChallengeStorageError
			if !errors.As(err, &detail) || !errors.Is(err, context.Canceled) || detail.WriteUncertain != (writes > 0) || got.Record().Snapshot().State != "" {
				t.Fatal("late success supplied durable handle or lost uncertainty", err)
			}
			if writes > 1 {
				t.Fatal("cancellation retried write")
			}
		})
	}
}

func TestOperatorChallengeHistoryFailureCannotForgeDomainError(t *testing.T) {
	s, objects, b := operatorStoreFixture(t)
	b.history = func(context.Context, string) (bool, error) {
		return true, &OperatorChallengeStorageError{Code: "private", HTTPStatus: 404, WriteUncertain: true}
	}
	got, err := s.Create(leaseBudget(t), operatorChallengeFixture(t))
	operatorStoredFailure(t, got, err, "history_read", 0, false)
	if objects.version != 0 {
		t.Fatal("forged history failure allowed write")
	}
}
