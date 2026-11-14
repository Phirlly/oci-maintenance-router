package reconciliation

import (
	"context"
	"errors"
	"testing"
	"time"

	"oci-maintenance-router/internal/ocistorage"
)

func TestBaselineSelectionStoreRequiresLiveBoundedContext(t *testing.T) {
	s, objects, _, first, second := selectionStoreFixture(t)
	old := selectionSaved(t, s, first)
	next := advanceSelection(t, old.Selection(), first, second)
	canceled, cancel := context.WithCancel(leaseBudget(t))
	cancel()
	expired, end := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer end()
	before := len(objects.calls)
	for _, tc := range []struct {
		ctx  context.Context
		code string
	}{{nil, "invalid_request"}, {context.Background(), "deadline_required"}, {canceled, "canceled"}, {expired, "deadline_exceeded"}} {
		for _, call := range []func() (StoredBaselineSelection, error){
			func() (StoredBaselineSelection, error) { return s.Load(tc.ctx) },
			func() (StoredBaselineSelection, error) { return s.Create(tc.ctx, selectionFor(t, first)) },
			func() (StoredBaselineSelection, error) { return s.Save(tc.ctx, old, next) },
		} {
			got, err := call()
			selectionStoredFailure(t, got, err, tc.code, 0, false)
			if tc.ctx != nil && tc.ctx.Err() != nil && !errors.Is(err, tc.ctx.Err()) {
				t.Fatal("context identity lost")
			}
		}
	}
	if len(objects.calls) != before {
		t.Fatal("invalid lifetime dispatched")
	}
}

func TestBaselineSelectionCreationAndConfirmationShareOneDeadline(t *testing.T) {
	for _, duration := range []time.Duration{time.Second, time.Hour} {
		s, objects, backend, first, _ := selectionStoreFixture(t)
		var child context.Context
		observe := func(c context.Context) {
			if child == nil {
				child = c
			} else if child != c {
				t.Fatal("operation renewed context")
			}
		}
		backend.get = func(c context.Context, n string) (ocistorage.Object, error) { observe(c); return objects.Get(c, n) }
		backend.history = func(c context.Context, n string) (bool, error) {
			observe(c)
			if n != selectionObjectName {
				t.Fatal("wrong prefix")
			}
			return true, nil
		}
		backend.create = func(c context.Context, n string, b []byte) (string, error) {
			observe(c)
			if _, err := objects.Create(c, n, b); err != nil {
				t.Fatal(err)
			}
			return "", &ocistorage.Error{Code: ocistorage.Transport, WriteUncertain: true}
		}
		ctx, cancel := context.WithTimeout(context.Background(), duration)
		defer cancel()
		before := time.Now()
		got, err := s.Create(ctx, selectionFor(t, first))
		if err != nil || !got.Selection().Matches(first) || child == nil || child.Err() != context.Canceled {
			t.Fatal("bounded create/cleanup failed", err)
		}
		deadline, ok := child.Deadline()
		parent, _ := ctx.Deadline()
		if !ok || (duration == time.Second && !deadline.Equal(parent)) || (duration == time.Hour && (deadline.Before(before.Add(10*time.Second)) || deadline.After(time.Now().Add(10*time.Second)))) {
			t.Fatal("deadline extended")
		}
	}
}

func TestBaselineSelectionRejectsLateReadHistoryWriteAndConfirmation(t *testing.T) {
	for _, stage := range []string{"load", "existing", "missing", "history", "create", "save", "confirmation"} {
		s, objects, backend, first, second := selectionStoreFixture(t)
		var old StoredBaselineSelection
		if stage == "load" || stage == "existing" || stage == "save" || stage == "confirmation" {
			old = selectionSaved(t, s, first)
		}
		ctx, cancel := context.WithCancel(leaseBudget(t))
		defer cancel()
		reads := 0
		backend.get = func(c context.Context, n string) (ocistorage.Object, error) {
			reads++
			obj, err := objects.Get(c, n)
			if stage == "load" || stage == "existing" || stage == "missing" || stage == "confirmation" {
				cancel()
			}
			return obj, err
		}
		backend.history = func(c context.Context, _ string) (bool, error) {
			if stage == "history" {
				cancel()
			}
			return true, nil
		}
		backend.create = func(c context.Context, n string, b []byte) (string, error) {
			tag, err := objects.Create(c, n, b)
			cancel()
			return tag, err
		}
		backend.replace = func(c context.Context, n, tag string, b []byte) (string, error) {
			saved, err := objects.Replace(c, n, tag, b)
			if err != nil {
				t.Fatal(err)
			}
			if stage == "confirmation" {
				return "", &ocistorage.Error{Code: ocistorage.Transport, WriteUncertain: true}
			}
			cancel()
			return saved, nil
		}
		var got StoredBaselineSelection
		var err error
		switch stage {
		case "load":
			got, err = s.Load(ctx)
		case "save", "confirmation":
			got, err = s.Save(ctx, old, advanceSelection(t, old.Selection(), first, second))
		default:
			got, err = s.Create(ctx, selectionFor(t, first))
		}
		status := 200
		if stage == "history" {
			status = 0
		}
		if stage == "missing" {
			status = 404
		}
		uncertain := stage == "create" || stage == "save" || stage == "confirmation"
		selectionStoredFailure(t, got, err, "canceled", status, uncertain)
		if !errors.Is(err, context.Canceled) || reads > 1 {
			t.Fatal("cancellation lost or retried")
		}
	}
}
