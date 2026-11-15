package deployment

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http/httptest"
	"strings"
	"testing"

	"oci-maintenance-router/internal/reconciliation"
)

func TestInvocationLogUsesFinalCleanupAndOnlySafeSummary(t *testing.T) {
	h, _, objects, _ := liveOperationFixture(t)
	var logs bytes.Buffer
	h.logger = log.New(&logs, "", 0)
	h.compose = func(Settings, Store, pinnedInputs, string) (operations, error) {
		ops := successfulReporting()
		ops.process = func(context.Context, *reconciliation.Lease) (reconciliation.ProcessingSnapshot, error) {
			objects.before = func(_ context.Context, method, _ string) error {
				if method == "REPLACE" {
					return errors.New("private-cleanup")
				}
				return nil
			}
			return reconciliation.ProcessingSnapshot{Completed: true, Accepted: 2,
				Issues:   []reconciliation.ProcessingIssue{{NoticeID: "private-notice", Code: "private-code"}},
				Recovery: &reconciliation.ProcessingRecovery{RequestID: "private-recovery", Status: "REJECTED"}}, nil
		}
		return ops, nil
	}
	h.Serve(operationalContext(t), strings.NewReader(`{"operation":"reconcile"}`), httptest.NewRecorder())
	var got map[string]any
	if json.Unmarshal(logs.Bytes(), &got) != nil || got["status"] != "not-ready" || got["cleanupCode"] != "release_failed" || got["scanCompleted"] != false || bytes.Count(logs.Bytes(), []byte("\n")) != 1 {
		t.Fatal("missing or premature finalized log", logs.String())
	}
	for _, private := range []string{"private-", "configurationSHA256", "releaseSHA256", "routingGeneration", "requestId", "receipt"} {
		if strings.Contains(logs.String(), private) {
			t.Fatal("log leaked private context", private)
		}
	}
	if got["processing"].(map[string]any)["accepted"] != float64(2) {
		t.Fatal("log lost accepted work")
	}
}

func TestRejectedInvocationLogContainsNoRequest(t *testing.T) {
	h := New(func(string) string { return "" })
	var logs bytes.Buffer
	h.logger = log.New(&logs, "", 0)
	h.Serve(operationalContext(t), strings.NewReader(`{"private-key":"private-value"}`), httptest.NewRecorder())
	var got map[string]any
	if json.Unmarshal(logs.Bytes(), &got) != nil || got["code"] != "request_invalid" || strings.Contains(logs.String(), "private") {
		t.Fatal("unsafe rejection log", logs.String())
	}
}
