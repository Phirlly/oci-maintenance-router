package reconciliation

import (
	"context"
	"errors"
	"testing"
	"time"

	"oci-maintenance-router/internal/ocistorage"
)

func TestBaselineStoreConfirmationSharesOriginalBudget(t *testing.T) {
	for _, late := range []bool{false, true} {
		_, s, b := baselineStoreFixture(t)
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		var writeDeadline time.Time
		backend := &baselineBackend{
			create: func(c context.Context, n string, body []byte) (string, error) {
				writeDeadline, _ = c.Deadline()
				if _, err := s.Create(c, n, body); err != nil {
					t.Fatal(err)
				}
				return "", &ocistorage.Error{Code: ocistorage.Transport, WriteUncertain: true}
			},
			get: func(c context.Context, n string) (ocistorage.Object, error) {
				deadline, _ := c.Deadline()
				if !deadline.Equal(writeDeadline) {
					t.Fatal("confirmation renewed the operation budget")
				}
				obj, err := s.Get(c, n)
				if late {
					cancel()
				}
				return obj, err
			},
		}
		got, err := baselineStoreFor(t, backend, b.EnvironmentID()).Create(ctx, b)
		if late {
			baselineStoredFailure(t, got, err, "canceled", 200, true)
		} else if err != nil || got.ID() != b.ID() {
			t.Fatal("bounded confirmation failed", err)
		}
		if len(s.calls) != 2 {
			t.Fatal("extra write or confirmation")
		}
	}
}

func TestBaselineStoreCancellationStopsInflightIOWithoutConfirmation(t *testing.T) {
	_, _, b := baselineStoreFixture(t)
	for _, create := range []bool{false, true} {
		ctx, cancel := context.WithCancel(leaseBudget(t))
		calls := 0
		stop := func(c context.Context) error {
			calls++
			cancel()
			select {
			case <-c.Done():
				return c.Err()
			case <-time.After(time.Second):
				t.Fatal("cancellation did not reach backend")
				return nil
			}
		}
		backend := &baselineBackend{
			get:    func(c context.Context, _ string) (ocistorage.Object, error) { return ocistorage.Object{}, stop(c) },
			create: func(c context.Context, _ string, _ []byte) (string, error) { return "", stop(c) },
		}
		store := baselineStoreFor(t, backend, b.EnvironmentID())
		var got Baseline
		var err error
		if create {
			got, err = store.Create(ctx, b)
		} else {
			got, err = store.Load(ctx, b.ID())
		}
		baselineStoredFailure(t, got, err, "canceled", 0, create)
		if calls != 1 || !errors.Is(err, context.Canceled) {
			t.Fatal("canceled I/O retried or lost cancellation")
		}
	}
}
