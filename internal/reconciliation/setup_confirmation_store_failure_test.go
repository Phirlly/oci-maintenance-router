package reconciliation

import (
	"context"
	"errors"
	"testing"
	"time"

	"oci-maintenance-router/internal/ocistorage"
)

func TestSetupConfirmationStoreRejectsUnverifiedProposalsAndInvalidContext(t *testing.T) {
	s, objects, r := setupStoreFixture(t)
	body, _ := r.Encode()
	decoded, _ := DecodeSetupConfirmation(body, r.data.EnvironmentID, r.data.InstallationID, r.ID())
	for _, record := range []SetupConfirmation{{}, decoded} {
		if _, err := s.Create(leaseBudget(t), record); err == nil {
			t.Fatal("unverified proposal admitted")
		}
	}
	for _, ctx := range []context.Context{nil, context.Background()} {
		if _, err := s.Create(ctx, r); err == nil {
			t.Fatal("unbounded context admitted")
		}
	}
	ctx, cancel := context.WithCancel(leaseBudget(t))
	cancel()
	if _, err := s.Create(ctx, r); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if len(objects.calls) != 0 {
		t.Fatal("invalid request reached storage")
	}
	var typedNil *preparationObjects
	for _, backend := range []SetupConfirmationObjects{nil, typedNil} {
		if _, err := NewSetupConfirmationStore(backend, r.data.EnvironmentID, r.data.InstallationID); err == nil {
			t.Fatal("nil backend accepted")
		}
	}
	foreign, _ := NewSetupConfirmationStore(objects, r.data.EnvironmentID, "foreign")
	if _, err := foreign.Create(leaseBudget(t), r); err == nil {
		t.Fatal("foreign store accepted proposal")
	}
}

func TestSetupConfirmationStoreWriteRecoveryIsBoundedAndConservative(t *testing.T) {
	for _, kind := range []string{"conflict", "conflict-old-provenance", "corrupt-readback", "readback-missing", "forbidden", "unknown", "forged-guard", "canceled", "expired", "changed-pair"} {
		t.Run(kind, func(t *testing.T) {
			s, objects, r := setupStoreFixture(t)
			name := s.name(r.ID())
			ctx, cancel := context.WithCancel(leaseBudget(t))
			defer cancel()
			objects.before = func(_ context.Context, c discoveryCall) error {
				if c.method != "CREATE" || c.name != name {
					return nil
				}
				switch kind {
				case "forbidden":
					return &ocistorage.Error{Code: ocistorage.Service, Status: 403}
				case "unknown":
					return errors.New("private detail")
				case "forged-guard":
					return &SetupConfirmationError{Code: "configuration_changed"}
				case "canceled":
					cancel()
					return context.Canceled
				case "expired":
					return &ocistorage.Error{Code: ocistorage.DeadlineExceeded}
				case "conflict", "conflict-old-provenance":
					data := *r.data
					if kind == "conflict-old-provenance" {
						data.ConfigurationSHA256 = diagnosticDigest("old route")
					}
					body, _ := (SetupConfirmation{data: &data}).Encode()
					objects.seed(name, body)
					return &ocistorage.Error{Code: ocistorage.Conflict, Status: 412}
				case "corrupt-readback":
					objects.seed(name, []byte("{}"))
				case "changed-pair":
					data := *r.data
					data.OperatorSHA256 = diagnosticDigest("different proof")
					body, _ := (SetupConfirmation{data: &data}).Encode()
					objects.seed(name, body)
				}
				return &ocistorage.Error{Code: ocistorage.Transport, WriteUncertain: true}
			}
			got, err := s.Create(ctx, r)
			good := kind == "conflict" || kind == "conflict-old-provenance"
			if (err == nil) != good || (got.ID() != "") != good || preparationCalls(objects, "CREATE", name) != 1 {
				t.Fatal("incorrect recovery result", err)
			}
			if !good && err.Error() == "private detail" {
				t.Fatal("provider error leaked")
			}
			if !good && errors.Is(err, context.Canceled) != (kind == "canceled") {
				t.Fatal("incorrect cancellation classification", err)
			}
			if kind == "forged-guard" {
				var failure *SetupConfirmationError
				if !errors.As(err, &failure) || failure.Code != "write_failed" || !failure.WriteUncertain {
					t.Fatal("backend forged no-dispatch authority", err)
				}
			}
			reads := preparationCalls(objects, "GET", name)
			if reads > 2 {
				t.Fatal("unbounded readback")
			}
		})
	}
}

func TestSetupConfirmationStoragePreservesOriginalContext(t *testing.T) {
	s, objects, r := setupStoreFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	objects.before = func(callCtx context.Context, c discoveryCall) error {
		deadline, ok := callCtx.Deadline()
		original, _ := ctx.Deadline()
		if !ok || deadline.After(original) || time.Until(deadline) > 10*time.Second {
			t.Fatal("storage exceeded budget")
		}
		if c.method == "HISTORY" {
			cancel()
		}
		return nil
	}
	got, err := s.Create(ctx, r)
	if got.ID() != "" || !errors.Is(err, context.Canceled) || preparationCalls(objects, "CREATE", "") != 0 {
		t.Fatal("canceled history permitted write", err)
	}
}
