package deployment

import (
	"context"
	"oci-maintenance-router/internal/ocistorage"
	"testing"
)

func TestCheckReconcilesOnlyAnIdenticalSavedMarker(t *testing.T) {
	s, store := fixture(t)
	if result := check(testContext(t), s, store, testAttemptID); result.Status != "deployment-verified" {
		t.Fatal(result)
	}
	for _, writeError := range []error{
		&ocistorage.Error{Code: ocistorage.Conflict},
		&ocistorage.Error{Code: ocistorage.Transport, WriteUncertain: true},
	} {
		store.createError = writeError
		if result := check(testContext(t), s, store, testAttemptID); result.Status != "deployment-verified" {
			t.Fatal(result)
		}
	}
	store.readOverride = []byte("different marker")
	if result := check(testContext(t), s, store, testAttemptID); result.Code != "storage_verify" {
		t.Fatal(result)
	}
	store.createError = &ocistorage.Error{Code: ocistorage.Authentication}
	if result := check(testContext(t), s, store, testAttemptID); result.Code != "storage_write" {
		t.Fatal(result)
	}
}

func TestCheckDoesNotHideExplicitWriteDenialBehindOldMarker(t *testing.T) {
	s, store := fixture(t)
	if result := check(testContext(t), s, store, testAttemptID); result.Status != "deployment-verified" {
		t.Fatal(result)
	}
	for _, status := range []int{400, 401, 403, 404, 429} {
		store.createError = &ocistorage.Error{Code: ocistorage.Service, Status: status, WriteUncertain: true}
		if result := check(testContext(t), s, store, testAttemptID); result.Code != "storage_write" {
			t.Fatalf("write denied with %d but check returned %+v", status, result)
		}
	}
}

func TestCheckDoesNotSucceedAfterCancellation(t *testing.T) {
	for _, phase := range []string{"before", "read", "write", "verify"} {
		s, store := fixture(t)
		ctx, cancel := context.WithCancel(testContext(t))
		defer cancel()
		wrapped := &cancelStore{memoryStore: store, cancel: cancel, phase: phase}
		if phase == "before" {
			cancel()
		}
		if got := check(ctx, s, wrapped, testAttemptID); got.Code != "deadline" || got.SendingEnabled {
			t.Fatalf("phase %s: %+v", phase, got)
		}
		if (phase == "before" || phase == "read") && store.creates != 0 {
			t.Fatal("late write")
		}
	}
}

type cancelStore struct {
	*memoryStore
	cancel context.CancelFunc
	phase  string
}

func (s *cancelStore) GetVersion(ctx context.Context, n, v string) (ocistorage.Object, error) {
	o, e := s.memoryStore.GetVersion(ctx, n, v)
	if s.phase == "read" {
		s.cancel()
	}
	return o, e
}
func (s *cancelStore) Create(ctx context.Context, n string, b []byte) (string, error) {
	v, e := s.memoryStore.Create(ctx, n, b)
	if s.phase == "write" {
		s.cancel()
	}
	return v, e
}
func (s *cancelStore) Get(ctx context.Context, n string) (ocistorage.Object, error) {
	o, e := s.memoryStore.Get(ctx, n)
	if s.phase == "verify" {
		s.cancel()
	}
	return o, e
}
