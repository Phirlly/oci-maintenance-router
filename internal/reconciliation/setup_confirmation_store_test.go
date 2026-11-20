package reconciliation

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"oci-maintenance-router/internal/ocistorage"
)

func setupStoreFixture(t *testing.T) (*SetupConfirmationStore, *preparationObjects, SetupConfirmation) {
	t.Helper()
	f, d, o := setupProofs(t)
	r, err := newSetupConfirmation(d, o, f.lease.manager.now())
	if err != nil {
		t.Fatal(err)
	}
	s, err := NewSetupConfirmationStore(f.objects, r.data.EnvironmentID, r.data.InstallationID)
	if err != nil {
		t.Fatal(err)
	}
	f.objects.calls = nil
	return s, f.objects, r
}

func TestSetupConfirmationStorePreservesFirstPairAndProvenance(t *testing.T) {
	s, objects, r := setupStoreFixture(t)
	first, err := s.Create(leaseBudget(t), r)
	if err != nil || first.ID() != r.ID() {
		t.Fatalf("first paired save failed: %v", err)
	}
	want, _ := first.Encode()
	copy := *r.data
	copy.ConfigurationSHA256 = diagnosticDigest("changed route configuration")
	copy.ConfirmedAt = r.confirmedAt().Add(time.Second).Format(leaseTimeFormat)
	r.data = &copy
	second, err := s.Create(leaseBudget(t), r)
	got, _ := second.Encode()
	if err != nil || !bytes.Equal(got, want) || preparationCalls(objects, "CREATE", s.name(r.ID())) != 1 || preparationCalls(objects, "HISTORY", s.name(r.ID())) != 1 {
		t.Fatal("repeat rewrote first confirmation provenance", err)
	}
}

func TestSetupConfirmationStoreRejectsMissingHistoryAndUntrustedReads(t *testing.T) {
	for _, kind := range []string{"retained", "history-failure", "forbidden", "unknown", "corrupt", "weak-etag", "future"} {
		t.Run(kind, func(t *testing.T) {
			s, objects, r := setupStoreFixture(t)
			name := s.name(r.ID())
			switch kind {
			case "retained":
				objects.history = func(context.Context, string) (bool, error) { return false, nil }
			case "history-failure":
				objects.history = func(context.Context, string) (bool, error) { return false, errors.New("private history") }
			case "forbidden", "unknown":
				objects.before = func(_ context.Context, c discoveryCall) error {
					if c.name == name && c.method == "GET" {
						if kind == "forbidden" {
							return &ocistorage.Error{Code: ocistorage.Service, Status: 403}
						}
						return errors.New("private transport")
					}
					return nil
				}
			case "corrupt":
				objects.seed(name, []byte("{}"))
			case "weak-etag":
				body, _ := r.Encode()
				objects.objects[name] = ocistorage.Object{Body: body, ETag: "W/old"}
			case "future":
				data := *r.data
				data.ConfirmedAt = r.confirmedAt().Add(time.Hour).Format(leaseTimeFormat)
				future := SetupConfirmation{data: &data}
				body, _ := future.Encode()
				objects.seed(name, body)
			}
			got, err := s.Create(leaseBudget(t), r)
			if err == nil || got.ID() != "" || preparationCalls(objects, "CREATE", name) != 0 {
				t.Fatal("unsafe missing/existing state admitted", err)
			}
		})
	}
}

func TestSetupConfirmationStoreConfirmsLostResponseWithoutSecondWrite(t *testing.T) {
	s, objects, r := setupStoreFixture(t)
	objects.after = func(_ context.Context, c discoveryCall) error {
		if c.name == s.name(r.ID()) && c.method == "CREATE" {
			return &ocistorage.Error{Code: ocistorage.Transport, WriteUncertain: true}
		}
		return nil
	}
	got, err := s.Create(leaseBudget(t), r)
	if err != nil || got.ID() != r.ID() || preparationCalls(objects, "CREATE", s.name(r.ID())) != 1 || preparationCalls(objects, "GET", s.name(r.ID())) != 2 {
		t.Fatal("exact uncertain write not confirmed", err)
	}
}
