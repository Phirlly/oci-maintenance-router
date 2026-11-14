package reconciliation

import (
	"context"
	"errors"
	"testing"
	"time"

	"oci-maintenance-router/internal/ocistorage"
)

func TestPreparedHistoryStoreRequiresLiveBoundedContext(t *testing.T) {
	s, objects, first, _ := preparedStoreFixture(t)
	old, err := s.Create(leaseBudget(t))
	if err != nil {
		t.Fatal(err)
	}
	next := advancePrepared(t, old.History(), Baseline{}, first)
	canceled, cancel := context.WithCancel(leaseBudget(t))
	cancel()
	expired, end := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer end()
	before := len(objects.calls)
	for _, tt := range []struct {
		ctx  context.Context
		code string
	}{{nil, "invalid_request"}, {context.Background(), "deadline_required"}, {canceled, "canceled"}, {expired, "deadline_exceeded"}} {
		for _, call := range []func() (StoredPreparedHistory, error){func() (StoredPreparedHistory, error) { return s.Load(tt.ctx) }, func() (StoredPreparedHistory, error) { return s.Create(tt.ctx) }, func() (StoredPreparedHistory, error) { return s.Save(tt.ctx, old, next) }} {
			got, err := call()
			preparedStoredFailure(t, got, err, tt.code, 0, false)
			if tt.ctx != nil && tt.ctx.Err() != nil && !errors.Is(err, tt.ctx.Err()) {
				t.Fatal("context identity lost")
			}
		}
	}
	if len(objects.calls) != before {
		t.Fatal("invalid lifetime dispatched")
	}
}

func TestPreparedHistoryStoreCapsDeadlineAndCancelsChild(t *testing.T) {
	for _, duration := range []time.Duration{time.Second, time.Hour} {
		_, _, first, _ := preparedStoreFixture(t)
		h, _ := NewPreparedHistory(first.EnvironmentID(), "installation-1")
		body, _ := h.Encode()
		var child context.Context
		backend := &preparedBackend{
			get: func(c context.Context, _ string) (ocistorage.Object, error) {
				child = c
				return ocistorage.Object{Body: body, ETag: "v1"}, nil
			},
			create:  func(c context.Context, _ string, _ []byte) (string, error) { child = c; return "v1", nil },
			replace: func(c context.Context, _, _ string, _ []byte) (string, error) { child = c; return "v2", nil },
		}
		s := preparedStoreFor(t, backend, first.EnvironmentID(), "installation-1")
		old, err := s.Create(leaseBudget(t))
		if err != nil {
			t.Fatal(err)
		}
		next := advancePrepared(t, old.History(), Baseline{}, first)
		ctx, cancel := context.WithTimeout(context.Background(), duration)
		defer cancel()
		for _, call := range []func() (StoredPreparedHistory, error){func() (StoredPreparedHistory, error) { return s.Load(ctx) }, func() (StoredPreparedHistory, error) { return s.Create(ctx) }, func() (StoredPreparedHistory, error) { return s.Save(ctx, old, next) }} {
			before := time.Now()
			_, err := call()
			if err != nil || child == nil || child.Err() != context.Canceled {
				t.Fatal("call/child cleanup failed", err)
			}
			deadline, ok := child.Deadline()
			parent, _ := ctx.Deadline()
			if !ok || (duration == time.Second && !deadline.Equal(parent)) || (duration == time.Hour && (deadline.Before(before.Add(10*time.Second)) || deadline.After(time.Now().Add(10*time.Second)))) {
				t.Fatal("deadline extended")
			}
		}
	}
}
