package deployment

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"reflect"
	"strings"
	"testing"

	"oci-maintenance-router/internal/reconciliation"
)

func TestWorkerDiagnosticRejectsUnknownAndTypedNilDetails(t *testing.T) {
	var absent *reconciliation.LeaseError
	for _, err := range []error{nil, errors.New("private-token"), absent,
		fmt.Errorf("private-wrapper: %w", absent),
		&reconciliation.LeaseError{Code: "private-code", HTTPStatus: 403, Request: "private-object"},
	} {
		got := describeWorkerFailure("acquire", err)
		if !reflect.DeepEqual(got, &workerDiagnostic{Stage: "acquire", Code: "unknown"}) {
			t.Fatal("unrecognized error exposed details", got)
		}
	}
	known := &reconciliation.LeaseError{Code: "write_failed", Request: "private-request",
		StorageCategory: "private-category", HTTPStatus: 503,
		PriorWrite: &reconciliation.LeaseWriteDiagnostic{Category: "private-prior", HTTPStatus: 403}}
	got := describeWorkerFailure("private-stage", fmt.Errorf("private-wrapper: %w", known))
	raw, _ := json.Marshal(got)
	if strings.Contains(string(raw), "private") || got.Stage != "unknown" || got.Request != "" || got.StorageCategory != "unknown" || got.HTTPStatus != 0 || got.PriorWrite.Category != "unknown" || got.PriorWrite.HTTPStatus != 0 {
		t.Fatal("private diagnostics escaped", string(raw))
	}
}

func TestWorkerDiagnosticAllowlistAndStatusBounds(t *testing.T) {
	for _, code := range []string{"invalid_request", "clock_invalid", "read_failed", "write_failed", "invalid_response", "ownership_lost", "conflict", "invalid_record", "canceled", "deadline_exceeded", "deadline_required", "insufficient_time"} {
		if describeWorkerFailure("acquire", &reconciliation.LeaseError{Code: code}).Code != code {
			t.Fatal("controlled lease code lost", code)
		}
	}
	for _, category := range []string{"invalid_configuration", "invalid_request", "deadline_required", "canceled", "deadline_exceeded", "authentication", "transport", "invalid_response", "too_large", "conflict", "service"} {
		for _, status := range []int{-1, 0, 99, 100, 599, 600, 12345678} {
			got := describeWorkerFailure("release", &reconciliation.LeaseError{Code: "write_failed", Request: "replace_confirmation", StorageCategory: category, HTTPStatus: status, WriteUncertain: true,
				PriorWrite: &reconciliation.LeaseWriteDiagnostic{Category: category, HTTPStatus: status}})
			want := 0
			if status >= 100 && status <= 599 {
				want = status
			}
			if got.StorageCategory != category || got.HTTPStatus != want || got.PriorWrite.Category != category || got.PriorWrite.HTTPStatus != want || !got.WriteUncertain || got.Request != "replace_confirmation" {
				t.Fatal("controlled storage evidence changed", got)
			}
		}
	}
	var nilGuard *ownershipGuardError
	for _, err := range []error{nil, nilGuard, errors.New("current_configuration_read")} {
		if describeOwnershipGuard("current_before", err) != nil {
			t.Fatal("unrelated guard acquired worker details")
		}
	}
}

func TestWorkerDiagnosticsStayOutOfResponsesAndUnrelatedLogs(t *testing.T) {
	diagnostic := describeWorkerFailure("acquire", &reconciliation.LeaseError{Code: "read_failed", Request: "read"})
	r := OperationalResult{Status: "not-ready", worker: diagnostic, workerCleanup: diagnostic}
	raw, _ := json.Marshal(r)
	for _, field := range []string{"worker", "stage", "request", "read_failed"} {
		if strings.Contains(string(raw), field) {
			t.Fatal("private diagnostic serialized in response", string(raw))
		}
	}
	for _, r := range []response{
		{legacy: Result{Status: "deployment-verified"}},
		{operational: &OperationalResult{Status: "operational-verified"}},
		{operational: &OperationalResult{Status: "coalesced"}},
		{operational: &OperationalResult{Status: "not-ready", Code: "operation_configuration"}},
	} {
		var logs bytes.Buffer
		h := &Handler{logger: log.New(&logs, "", 0)}
		h.logResult(r)
		if strings.Contains(logs.String(), "worker") {
			t.Fatal("unrelated outcome acquired worker diagnostic", logs.String())
		}
	}
}
