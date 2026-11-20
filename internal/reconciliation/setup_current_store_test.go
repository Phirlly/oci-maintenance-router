package reconciliation

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"oci-maintenance-router/internal/ocistorage"
)

func setupCurrentStoreFixture(t *testing.T) (*setupCurrentStore, *preparationObjects, setupCurrent) {
	t.Helper()
	_, objects, _ := preparationFixture(t, nil)
	d, o := setupCurrentIntents(t, 1)
	r, err := newSetupCurrent(1, d, o)
	if err != nil {
		t.Fatal(err)
	}
	s, err := newSetupCurrentStore(objects, d.EnvironmentID, d.InstallationID)
	if err != nil {
		t.Fatal(err)
	}
	objects.calls = nil
	return s, objects, r
}

func TestSetupCurrentStoreUsesExactConditionalPredecessor(t *testing.T) {
	s, objects, r := setupCurrentStoreFixture(t)
	first, err := s.create(leaseBudget(t), r)
	if err != nil || first.record.data == nil {
		t.Fatal("first selection failed", err)
	}
	d, o := setupCurrentIntents(t, 2)
	next, err := first.record.selectAttempt(2, d, o)
	if err != nil {
		t.Fatal(err)
	}
	saved, err := s.save(leaseBudget(t), first, next)
	if err != nil || saved.record.data.Attempt != 2 {
		t.Fatal("conditional selection failed", err)
	}
	loaded, missing, err := s.load(leaseBudget(t))
	if err != nil || missing || loaded.record.data.Attempt != 2 {
		t.Fatal("selection did not survive load", err)
	}
	if _, err := s.save(leaseBudget(t), loaded, next); err == nil {
		t.Fatal("fresh ETag accepted stale proposal")
	}
	if _, err := s.create(leaseBudget(t), r); err == nil {
		t.Fatal("Create replaced advanced selection")
	}
	if preparationCalls(objects, "CREATE", s.name()) != 1 || preparationCalls(objects, "REPLACE", s.name()) != 1 {
		t.Fatal("unwanted state mutation")
	}
}

func TestSetupCurrentStoreProtectsRetainedAndUntrustedMissingState(t *testing.T) {
	for _, kind := range []string{"retained", "history-error", "unknown404", "forbidden", "corrupt", "weak-tag"} {
		t.Run(kind, func(t *testing.T) {
			s, objects, r := setupCurrentStoreFixture(t)
			switch kind {
			case "retained":
				objects.history = func(context.Context, string) (bool, error) { return false, nil }
			case "history-error":
				objects.history = func(context.Context, string) (bool, error) { return false, errors.New("private") }
			case "unknown404", "forbidden":
				objects.before = func(_ context.Context, c discoveryCall) error {
					if c.method == "GET" && c.name == s.name() {
						if kind == "forbidden" {
							return &ocistorage.Error{Code: ocistorage.Service, Status: 403}
						}
						return errors.New("404 private")
					}
					return nil
				}
			case "corrupt":
				objects.seed(s.name(), []byte("{}"))
			case "weak-tag":
				body, _ := r.encode()
				objects.objects[s.name()] = ocistorage.Object{Body: body, ETag: "W/weak"}
			}
			if got, err := s.create(leaseBudget(t), r); err == nil || got.record.data != nil || preparationCalls(objects, "CREATE", s.name()) != 0 {
				t.Fatal("unsafe missing state recreated", err)
			}
		})
	}
}

func TestSetupCurrentStoreConfirmsOnlyExactLostWrite(t *testing.T) {
	for _, replace := range []bool{false, true} {
		s, objects, r := setupCurrentStoreFixture(t)
		var previous storedSetupCurrent
		if replace {
			var err error
			previous, err = s.create(leaseBudget(t), r)
			if err != nil {
				t.Fatal(err)
			}
			d, o := setupCurrentIntents(t, 2)
			r, err = previous.record.selectAttempt(2, d, o)
			if err != nil {
				t.Fatal(err)
			}
		}
		objects.after = func(_ context.Context, c discoveryCall) error {
			if c.name == s.name() && (c.method == "CREATE" || c.method == "REPLACE") {
				return &ocistorage.Error{Code: ocistorage.Transport, WriteUncertain: true}
			}
			return nil
		}
		var got storedSetupCurrent
		var err error
		if replace {
			got, err = s.save(leaseBudget(t), previous, r)
		} else {
			got, err = s.create(leaseBudget(t), r)
		}
		want, _ := r.encode()
		actual, _ := got.record.encode()
		if err != nil || !bytes.Equal(want, actual) {
			t.Fatal("exact uncertain write not confirmed", err)
		}
		if preparationCalls(objects, "CREATE", s.name()) != 1 || preparationCalls(objects, "REPLACE", s.name()) != map[bool]int{false: 0, true: 1}[replace] {
			t.Fatal("uncertain write retried")
		}
	}
}
