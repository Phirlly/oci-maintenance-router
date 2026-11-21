package deployment

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"net/http/httptest"
	"strings"
	"testing"

	"oci-maintenance-router/internal/ocistorage"
)

func TestWorkerFailureLogIdentifiesStageAndPreservesCleanup(t *testing.T) {
	for _, stage := range []string{"acquire", "current_before", "current_after", "release", "both"} {
		t.Run(stage, func(t *testing.T) {
			h, s, objects, _ := operationalFixture(t)
			s = configureOperational(t, h, s, objects, func(v map[string]any) { v["mode"] = "PAUSED" })
			composed := false
			objects.before = func(_ context.Context, method, name string) error {
				if name != "control/lease.json" {
					return nil
				}
				if stage == "acquire" && method == "CREATE" || (stage == "current_before" || stage == "both" || stage == "current_after" && composed) && method == "GET" {
					return &ocistorage.Error{Code: ocistorage.Service, Status: 403}
				}
				if (stage == "release" || stage == "both") && method == "REPLACE" {
					return &ocistorage.Error{Code: ocistorage.Service, Status: 429}
				}
				return nil
			}
			h.compose = func(s Settings, store Store, p pinnedInputs, operation string) (operations, error) {
				composed = true
				return composeOperations(s, store, p, operation)
			}
			var logs bytes.Buffer
			h.logger = log.New(&logs, "", 0)
			out := httptest.NewRecorder()
			h.Serve(operationalContext(t), strings.NewReader(invocation(s)), out)
			var got, response map[string]any
			if json.Unmarshal(logs.Bytes(), &got) != nil || json.Unmarshal(out.Body.Bytes(), &response) != nil || got["status"] != "not-ready" || bytes.Count(logs.Bytes(), []byte("\n")) != 1 {
				t.Fatal("missing final failure", logs.String(), out.Body.String())
			}
			if response["worker"] != nil || response["workerCleanup"] != nil {
				t.Fatal("diagnostic changed response contract", out.Body.String())
			}
			if composed != (stage == "current_after" || stage == "release") {
				t.Fatal("ownership failure did not guard composition")
			}
			if stage != "release" {
				want := stage
				if want == "both" {
					want = "current_before"
				}
				assertWorkerLog(t, got["worker"], want, 403)
			} else if got["worker"] != nil {
				t.Fatal("cleanup invented a primary failure")
			}
			if stage == "release" || stage == "both" {
				assertWorkerLog(t, got["workerCleanup"], "release", 429)
			} else if got["workerCleanup"] != nil {
				t.Fatal("invented cleanup failure")
			}
			for _, private := range []string{"control/lease", "ownerToken", "ocid1.", "configurationSha256", "releaseSha256"} {
				if strings.Contains(logs.String(), private) {
					t.Fatal("private context in log", private)
				}
			}
		})
	}
}

func assertWorkerLog(t *testing.T, value any, stage string, status int) {
	t.Helper()
	got, ok := value.(map[string]any)
	request, code := "read", "read_failed"
	if stage == "acquire" {
		request, code = "create", "write_failed"
	} else if stage == "release" {
		request, code = "replace", "write_failed"
	}
	if !ok || got["stage"] != stage || got["code"] != code || got["request"] != request || got["storageCategory"] != "service" || got["httpStatus"] != float64(status) {
		t.Fatal("missing controlled worker diagnostic", value)
	}
}

func TestWorkerLogRetainsUncertainWriteAndFailedConfirmation(t *testing.T) {
	h, s, objects, _ := operationalFixture(t)
	var calls []string
	objects.before = func(_ context.Context, method, name string) error {
		if name == "control/lease.json" {
			calls = append(calls, method)
			if method == "CREATE" {
				return &ocistorage.Error{Code: ocistorage.Service, Status: 503, WriteUncertain: true}
			}
		}
		return nil
	}
	var logs bytes.Buffer
	h.logger = log.New(&logs, "", 0)
	h.Serve(operationalContext(t), strings.NewReader(invocation(s)), httptest.NewRecorder())
	var got struct {
		Worker *workerDiagnostic `json:"worker"`
	}
	if json.Unmarshal(logs.Bytes(), &got) != nil || got.Worker == nil {
		t.Fatal("missing uncertainty diagnostic", logs.String())
	}
	w := got.Worker
	if w.Stage != "acquire" || w.Code != "read_failed" || w.Request != "create_confirmation" || w.StorageCategory != "service" || w.HTTPStatus != 404 || !w.WriteUncertain || w.PriorWrite == nil || w.PriorWrite.Category != "service" || w.PriorWrite.HTTPStatus != 503 {
		t.Fatal("initiating and confirmation failures were conflated", logs.String())
	}
	if strings.Join(calls, ",") != "CREATE,GET" || objects.version != 0 {
		t.Fatal("diagnostics retried or changed storage", calls, objects.version)
	}
}
