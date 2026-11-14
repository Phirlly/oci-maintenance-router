package reconciliation

import (
	"context"
	"errors"
	"testing"
	"time"

	"oci-maintenance-router/internal/ocistorage"
)

func TestBaselineStoreRequiresLiveBoundedLifetime(t *testing.T) {
	store, s, b := baselineStoreFixture(t)
	canceled, cancel := context.WithCancel(leaseBudget(t))
	cancel()
	expired, end := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer end()
	for _, tt := range []struct {
		ctx  context.Context
		code string
	}{
		{nil, "invalid_request"}, {context.Background(), "deadline_required"},
		{canceled, "canceled"}, {expired, "deadline_exceeded"},
	} {
		for _, create := range []bool{false, true} {
			var got Baseline
			var err error
			if create {
				got, err = store.Create(tt.ctx, b)
			} else {
				got, err = store.Load(tt.ctx, b.ID())
			}
			baselineStoredFailure(t, got, err, tt.code, 0, false)
			if tt.ctx != nil && tt.ctx.Err() != nil && !errors.Is(err, tt.ctx.Err()) {
				t.Fatal("context identity lost")
			}
		}
	}
	if len(s.calls) != 0 {
		t.Fatal("invalid lifetime dispatched")
	}
}

func TestBaselineStorePreservesShortDeadlineCapsLongAndCancelsChild(t *testing.T) {
	_, _, b := baselineStoreFixture(t)
	body, _ := b.Encode()
	for _, duration := range []time.Duration{time.Second, time.Hour} {
		for _, create := range []bool{false, true} {
			ctx, cancel := context.WithTimeout(context.Background(), duration)
			defer cancel()
			var child context.Context
			backend := &baselineBackend{
				get: func(c context.Context, _ string) (ocistorage.Object, error) {
					child = c
					return ocistorage.Object{Body: body, ETag: `"tag"`}, nil
				},
				create: func(c context.Context, _ string, _ []byte) (string, error) {
					child = c
					return `"tag"`, nil
				},
			}
			store := baselineStoreFor(t, backend, b.EnvironmentID())
			before := time.Now()
			var err error
			if create {
				_, err = store.Create(ctx, b)
			} else {
				_, err = store.Load(ctx, b.ID())
			}
			if err != nil || child == nil || !errors.Is(child.Err(), context.Canceled) {
				t.Fatal("successful call or child cleanup failed", err)
			}
			deadline, ok := child.Deadline()
			parent, _ := ctx.Deadline()
			if !ok || (duration == time.Second && !deadline.Equal(parent)) ||
				(duration == time.Hour && (deadline.Before(before.Add(10*time.Second)) || deadline.After(time.Now().Add(10*time.Second)))) {
				t.Fatal("operation extended caller or exceeded cap")
			}
		}
	}
}

func TestBaselineStoreRejectsLateSuccessfulReadAndWrite(t *testing.T) {
	_, _, b := baselineStoreFixture(t)
	body, _ := b.Encode()
	for _, create := range []bool{false, true} {
		ctx, cancel := context.WithCancel(leaseBudget(t))
		backend := &baselineBackend{
			get: func(context.Context, string) (ocistorage.Object, error) {
				cancel()
				return ocistorage.Object{Body: body, ETag: "v1"}, nil
			},
			create: func(context.Context, string, []byte) (string, error) { cancel(); return "v1", nil },
		}
		store := baselineStoreFor(t, backend, b.EnvironmentID())
		var got Baseline
		var err error
		if create {
			got, err = store.Create(ctx, b)
		} else {
			got, err = store.Load(ctx, b.ID())
		}
		baselineStoredFailure(t, got, err, "canceled", 200, create)
		if !errors.Is(err, context.Canceled) {
			t.Fatal("late cancellation identity lost")
		}
	}
}
