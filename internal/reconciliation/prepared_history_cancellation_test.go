package reconciliation

import (
	"context"
	"errors"
	"testing"
	"time"

	"oci-maintenance-router/internal/ocistorage"
)

func TestPreparedHistoryStoreRejectsCanceledReadAndWriteCompletion(t *testing.T) {
	for _, late := range []bool{false, true} {
		for _, operation := range []string{"load", "create", "save"} {
			_, _, first, _ := preparedStoreFixture(t)
			backend := &preparedBackend{create: func(context.Context, string, []byte) (string, error) { return "v1", nil }}
			s := preparedStoreFor(t, backend, first.EnvironmentID(), "installation-1")
			old, err := s.Create(leaseBudget(t))
			if err != nil {
				t.Fatal(err)
			}
			next := advancePrepared(t, old.History(), Baseline{}, first)
			body, _ := old.History().Encode()
			ctx, cancel := context.WithCancel(leaseBudget(t))
			calls := 0
			stop := func(c context.Context) error {
				calls++
				cancel()
				if late {
					return nil
				}
				select {
				case <-c.Done():
					return c.Err()
				case <-time.After(time.Second):
					t.Fatal("cancel did not reach backend")
					return nil
				}
			}
			backend.get = func(c context.Context, _ string) (ocistorage.Object, error) {
				return ocistorage.Object{Body: body, ETag: "v1"}, stop(c)
			}
			backend.create = func(c context.Context, _ string, _ []byte) (string, error) { return "v1", stop(c) }
			backend.replace = func(c context.Context, _, _ string, _ []byte) (string, error) { return "v2", stop(c) }
			var got StoredPreparedHistory
			switch operation {
			case "load":
				got, err = s.Load(ctx)
			case "create":
				got, err = s.Create(ctx)
			case "save":
				got, err = s.Save(ctx, old, next)
			}
			status := 0
			if late {
				status = 200
			}
			preparedStoredFailure(t, got, err, "canceled", status, operation != "load")
			if calls != 1 || !errors.Is(err, context.Canceled) {
				t.Fatal("canceled call retried or lost context identity")
			}
		}
	}
}

func TestPreparedHistoryConfirmationSharesBudgetAndRejectsLateSuccess(t *testing.T) {
	for _, create := range []bool{false, true} {
		for _, late := range []bool{false, true} {
			_, objects, first, _ := preparedStoreFixture(t)
			backend := &preparedBackend{create: objects.Create, get: objects.Get}
			s := preparedStoreFor(t, backend, first.EnvironmentID(), "installation-1")
			var old StoredPreparedHistory
			var next PreparedHistory
			if !create {
				var err error
				old, err = s.Create(leaseBudget(t))
				if err != nil {
					t.Fatal(err)
				}
				next = advancePrepared(t, old.History(), Baseline{}, first)
			}
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			var written context.Context
			backend.create = func(c context.Context, n string, b []byte) (string, error) {
				written = c
				_, err := objects.Create(c, n, b)
				if err != nil {
					t.Fatal(err)
				}
				return "", &ocistorage.Error{Code: ocistorage.Transport, WriteUncertain: true}
			}
			backend.replace = func(c context.Context, n, tag string, b []byte) (string, error) {
				written = c
				_, err := objects.Replace(c, n, tag, b)
				if err != nil {
					t.Fatal(err)
				}
				return "", &ocistorage.Error{Code: ocistorage.Transport, WriteUncertain: true}
			}
			reads := 0
			backend.get = func(c context.Context, n string) (ocistorage.Object, error) {
				reads++
				if c != written {
					t.Fatal("confirmation reset context")
				}
				obj, err := objects.Get(c, n)
				if late {
					cancel()
				}
				return obj, err
			}
			var got StoredPreparedHistory
			var err error
			if create {
				got, err = s.Create(ctx)
			} else {
				got, err = s.Save(ctx, old, next)
			}
			if late {
				preparedStoredFailure(t, got, err, "canceled", 200, true)
			} else if err != nil || got.History().EnvironmentID() != first.EnvironmentID() {
				t.Fatal("confirmation failed", err)
			}
			if reads != 1 {
				t.Fatal("confirmation retried")
			}
		}
	}
}
