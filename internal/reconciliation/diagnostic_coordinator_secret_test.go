package reconciliation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"oci-maintenance-router/internal/ocisecrets"
	"oci-maintenance-router/internal/pagerduty"
)

func TestDiagnosticCoordinatorRejectsUnusableCredentialsBeforeClaim(t *testing.T) {
	for _, kind := range []string{"nil", "zero-version", "error", "forged", "wrapped", "service", "invalid-status"} {
		t.Run(kind, func(t *testing.T) {
			f := coordinatorFixture(t)
			f.coordinator.current = func(context.Context, string) (diagnosticCredential, error) {
				switch kind {
				case "nil":
					return nil, nil
				case "zero-version":
					return diagnosticMaterial{0, func() []byte { t.Fatal("invalid bundle content read"); return nil }}, nil
				case "forged":
					return nil, &DiagnosticRunError{Code: "private-credential-text", HTTPStatus: 202}
				case "wrapped":
					return nil, fmt.Errorf("private-credential-text: %w", &ocisecrets.Error{Code: ocisecrets.Service, Status: 403})
				case "service":
					return nil, &ocisecrets.Error{Code: ocisecrets.Code("private-credential-text"), Status: 403}
				case "invalid-status":
					return nil, &ocisecrets.Error{Code: ocisecrets.Service, Status: 999}
				default:
					return nil, errors.New("private-credential-text")
				}
			}
			got, err := f.coordinator.Run(leaseBudget(t), f.lease)
			want := "secret_read"
			if kind == "nil" || kind == "zero-version" {
				want = "secret_invalid"
			}
			e := diagnosticRunFailure(t, err, want)
			status := 0
			if kind == "service" {
				status = 403
			}
			if e.HTTPStatus != status || got.Dispatched() || f.sends.Load() != 0 || preparationCalls(f.objects, "CREATE", diagnosticObjectName) != 0 || strings.Contains(fmt.Sprintf("%+v %#v", got, err), "private-credential-text") {
				t.Fatal("unusable credential leaked or created a claim")
			}
		})
	}
}

func TestDiagnosticCoordinatorKeepsCredentialBytesTransient(t *testing.T) {
	f := coordinatorFixture(t)
	const key = "synthetic-sensitive-key"
	var transient []byte
	f.coordinator.current = func(context.Context, string) (diagnosticCredential, error) {
		return diagnosticMaterial{7, func() []byte { return []byte(key) }}, nil
	}
	f.coordinator.send = func(_ context.Context, _, k []byte) (pagerduty.Response, error) {
		transient = k
		if string(k) != key {
			t.Error("wrong key")
		}
		return pagerduty.Response{}, errors.New(key)
	}
	got, err := f.coordinator.Run(leaseBudget(t), f.lease)
	if err != nil || got.Record().Snapshot().State != DiagnosticReview {
		t.Fatal(err)
	}
	for _, b := range transient {
		if b != 0 {
			t.Fatal("transient copy not cleared")
		}
	}
	encoded, _ := json.Marshal(got)
	if strings.Contains(fmt.Sprintf("%+v %#v %s", got, got, encoded), key) {
		t.Fatal("result exposed key")
	}
	for _, object := range f.objects.objects {
		if strings.Contains(string(object.Body), key) {
			t.Fatal("secret entered durable state")
		}
	}
}
