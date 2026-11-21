package reconciliation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"oci-maintenance-router/internal/ocistorage"
)

func TestLeaseStorageDiagnosticSanitizesErrorsAndPreservesCancellation(t *testing.T) {
	var absent *ocistorage.Error
	for _, source := range []error{errors.New("private provider message"), absent,
		fmt.Errorf("private wrapper: %w", absent),
		&ocistorage.Error{Code: "private-category", Status: 503},
	} {
		got := leaseError(context.Background(), "write_failed", source)
		raw, _ := json.Marshal(got)
		if strings.Contains(string(raw), "private") || strings.Contains(got.Error(), "private") {
			t.Fatal("unsafe storage diagnostic", string(raw))
		}
	}
	for _, category := range []ocistorage.Code{ocistorage.InvalidConfiguration, ocistorage.InvalidRequest, ocistorage.DeadlineRequired, ocistorage.Canceled, ocistorage.DeadlineExceeded, ocistorage.Authentication, ocistorage.Transport, ocistorage.InvalidResponse, ocistorage.TooLarge, ocistorage.Conflict, ocistorage.Service} {
		if leaseStorageCategory(category) != string(category) {
			t.Fatal("controlled category lost", category)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	got := leaseError(ctx, "write_failed", &ocistorage.Error{Code: ocistorage.Service, Status: 503, WriteUncertain: true})
	if !errors.Is(got, context.Canceled) || got.Code != "canceled" || got.StorageCategory != "service" || got.HTTPStatus != 503 || !got.WriteUncertain {
		t.Fatal("cancellation replaced safe causal evidence", got)
	}
}

func TestLeaseRequestEvidencePreservesFailuresAndOwnsItsCopy(t *testing.T) {
	var absent *LeaseError
	for _, err := range []error{nil, absent, errors.New("unrelated")} {
		if leaseRequestEvidence(err, "read", nil) != err {
			t.Fatal("unrelated error identity changed")
		}
	}
	original := &LeaseError{Code: "ownership_lost"}
	prior := uncertainLeaseWrite(&ocistorage.Error{Code: ocistorage.Service, Status: 503, WriteUncertain: true})
	got := leaseRequestEvidence(original, "replace_confirmation", prior).(*LeaseError)
	if original.Request != "" || original.PriorWrite != nil || got == original || got.Request != "replace_confirmation" || !got.WriteUncertain || got.PriorWrite.Category != "service" || got.PriorWrite.HTTPStatus != 503 {
		t.Fatal("evidence changed original failure or lost prior write")
	}
	for _, status := range []int{-1, 99, 600} {
		if uncertainLeaseWrite(&ocistorage.Error{Code: ocistorage.Service, Status: status}).HTTPStatus != 0 {
			t.Fatal("invalid prior status retained")
		}
	}
	if got := uncertainLeaseWrite(&ocistorage.Error{Code: "private-category", Status: 503}); got.Category != "unknown" || got.HTTPStatus != 0 {
		t.Fatal("unknown prior error retained details", got)
	}
}

func TestLeaseReadValidationKeepsRequestContext(t *testing.T) {
	for _, malformed := range []bool{false, true} {
		s := &leaseMemory{}
		f := leaseFunctionsFor(s)
		f.get = func(context.Context, string) (ocistorage.Object, error) {
			if malformed {
				return ocistorage.Object{Body: []byte("private malformed record"), ETag: "etag"}, nil
			}
			return ocistorage.Object{}, nil
		}
		_, _, err := leaseManager(t, f, leaseTime()).load(leaseBudget(t))
		var detail *LeaseError
		if !errors.As(err, &detail) || detail.Request != "read" || detail.StorageCategory != "" || strings.Contains(err.Error(), "private") {
			t.Fatal("read validation lost safe request context", err)
		}
	}
}
