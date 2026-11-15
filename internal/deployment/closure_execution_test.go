package deployment

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"oci-maintenance-router/internal/noticeledger"
)

func closeInvocation(s Settings, id string) string {
	b, _ := json.Marshal(map[string]string{"operation": "close-work", "configurationSha256": s.ConfigurationSHA256, "releaseSha256": s.ReleaseSHA256,
		"checkAttemptId": "12345678-1234-1234-1234-123456789abc", "recoveryRequestId": id})
	return string(b)
}

func TestExplicitClosurePersistsAndReusesWithoutServices(t *testing.T) {
	h, s, objects, env, principal := retainedTestFixture(t, false)
	s = configureOperational(t, h, s, objects, func(v map[string]any) { v["mode"] = "PAUSED" })
	s, store, notice := configureRecovery(t, h, s, objects, env, noticeledger.CloseRevision)
	var prior []byte
	for i := 0; i < 2; i++ {
		got := invokeOperational(t, h, closeInvocation(s, "recovery-1"))
		if got.Status != "closure-completed" || got.Recovery == nil || got.Recovery.Status != "CLOSED" || got.Processing == nil || *got.Processing != (ProcessingOutcome{}) || got.ScanCompleted || got.Diagnostic != nil || got.Operator != nil || principal.reads != 0 {
			t.Fatalf("closure failed or did unrelated work: %+v", got)
		}
		saved, err := store.Load(operationalContext(t), notice)
		if err != nil {
			t.Fatal(err)
		}
		current, _ := saved.Record().Encode()
		if i == 1 && !bytes.Equal(prior, current) {
			t.Fatal("repeat closure rewrote history")
		}
		prior = current
	}
	// Historical outcome is discoverable after the configured request is removed.
	s = configureOperational(t, h, s, objects, func(v map[string]any) { delete(v, "recovery") })
	got := closureResponse(t, h, closureReviewRequest(notice, "", "", ""))
	body, _ := json.Marshal(got)
	if got["status"] != "reviewed" || !strings.Contains(string(body), `"requestId":"recovery-1"`) || !strings.Contains(string(body), `"status":"CLOSED"`) {
		t.Fatal("closure history hidden", got)
	}
}
