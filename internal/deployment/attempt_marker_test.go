package deployment

import (
	"bytes"
	"strings"
	"testing"

	"oci-maintenance-router/internal/ocistorage"
)

const nextAttemptID = "fedcba98-7654-3210-fedc-ba9876543210"

func TestAttemptsKeepSeparateMarkersAndStableRouting(t *testing.T) {
	h, s, store := handlerFixture(t)
	var generation string
	for i, id := range []string{testAttemptID, nextAttemptID, testAttemptID} {
		result, echo := attemptResponse(t, h, s, id)
		if result.Status != "deployment-verified" || echo != id {
			t.Fatalf("attempt %d: %+v echo=%q", i, result, echo)
		}
		if i == 0 {
			generation = result.RoutingGeneration
		}
		if len(generation) != 64 || result.RoutingGeneration != generation || result.ConfigurationSHA256 != s.ConfigurationSHA256 || result.ReleaseSHA256 != s.ReleaseSHA256 {
			t.Fatalf("attempt changed trusted identity: %+v", result)
		}
	}
	if len(store.markers) != 2 || store.creates != 3 {
		t.Fatalf("markers=%d writes=%d", len(store.markers), store.creates)
	}
	for name, body := range store.markers {
		if !strings.HasPrefix(name, "deployment-checks/") || !strings.HasSuffix(name, ".json") || len(name) != len("deployment-checks/")+64+len(".json") {
			t.Fatalf("invalid marker path %q", name)
		}
		if len(body) > 1024 || !(bytes.Contains(body, []byte(testAttemptID)) || bytes.Contains(body, []byte(nextAttemptID))) {
			t.Fatalf("unbound marker %q", body)
		}
	}
}

func TestNewAttemptCannotRecoverFromPreviousMarker(t *testing.T) {
	for _, code := range []ocistorage.Code{ocistorage.Conflict, ocistorage.Transport} {
		t.Run(string(code), func(t *testing.T) {
			h, s, store := handlerFixture(t)
			if result, _ := attemptResponse(t, h, s, testAttemptID); result.Status != "deployment-verified" {
				t.Fatal(result)
			}
			store.createError = &ocistorage.Error{Code: code, WriteUncertain: code == ocistorage.Transport}
			result, echo := attemptResponse(t, h, s, nextAttemptID)
			if result.Code != "storage_verify" || result.Status != "not-ready" || echo != nextAttemptID || len(store.markers) != 1 {
				t.Fatalf("previous marker satisfied new attempt: %+v echo=%q", result, echo)
			}
		})
	}
}

func TestAttemptReconcilesSavedWriteWithLostResponse(t *testing.T) {
	h, s, store := handlerFixture(t)
	store.createError = &ocistorage.Error{Code: ocistorage.Transport, WriteUncertain: true}
	store.saveBeforeError = true
	for i := 0; i < 2; i++ {
		result, echo := attemptResponse(t, h, s, testAttemptID)
		if result.Status != "deployment-verified" || echo != testAttemptID || len(store.markers) != 1 {
			t.Fatalf("lost response: %+v echo=%q", result, echo)
		}
	}
}

func TestFreshAttemptRecoversAfterDeniedWrite(t *testing.T) {
	h, s, store := handlerFixture(t)
	store.createError = &ocistorage.Error{Code: ocistorage.Service, Status: 403, WriteUncertain: true}
	result, echo := attemptResponse(t, h, s, testAttemptID)
	if result.Code != "storage_write" || echo != testAttemptID || len(store.markers) != 0 {
		t.Fatalf("denial hidden: %+v echo=%q", result, echo)
	}
	store.createError = nil
	result, echo = attemptResponse(t, h, s, nextAttemptID)
	if result.Status != "deployment-verified" || echo != nextAttemptID || len(store.markers) != 1 || store.creates != 2 {
		t.Fatalf("recovery failed: %+v echo=%q", result, echo)
	}
}
