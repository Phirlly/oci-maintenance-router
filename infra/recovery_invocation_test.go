package infra_test

import (
	"encoding/json"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

func recoveryResult(kind string) map[string]any {
	v := closureResult()
	v["status"], v["operation"], v["mode"], v["scanCompleted"] = "recovery-verified", "recover-work", "LIVE", true
	status := "API_ACCEPTED"
	if kind == "APPROVE_REVISION" {
		status = "APPROVED"
	}
	v["recovery"] = map[string]any{"requestId": "r1-request", "status": status, "ambiguous": false}
	v["telemetry"] = map[string]any{"accepted": true}
	v["reporting"] = map[string]any{"known": true, "issues": 1, "reviewed": 1, "remaining": 0, "deferred": 1, "pending": true, "dispatched": false, "accepted": false}
	return v
}

func TestRecoveryInvocationRejectsStalePartialAndOtherWork(t *testing.T) {
	source, err := os.ReadFile("recovery.tf")
	if err != nil {
		t.Fatal(err)
	}
	match := regexp.MustCompile(`(?s)postcondition\s*\{\s*condition\s*=\s*(.*?)\s*error_message\s*=`).FindSubmatch(source)
	output := regexp.MustCompile(`(?s)recovery_verified\s*=\s*(.*?)\n}`).FindSubmatch(source)
	if len(match) != 2 || len(output) != 2 {
		t.Fatal("missing recovery acceptance predicates")
	}
	for _, action := range []string{"RETRY_BATCH", "APPROVE_REVISION"} {
		for _, kind := range []string{"valid", "ambiguous", "version", "operation", "mode", "status", "attempt", "configuration", "release", "generation", "prepared", "selected", "scan", "code", "cleanup", "diagnostic", "operator", "review", "savedNotices", "closureReview", "recoveryReview", "missing-processing", "processing-count", "processing-type", "processing-negative", "missing-reporting", "report-count", "report-type", "report-error", "missing-telemetry", "metric-error", "metric-rejected", "request", "wrong-action", "reason", "invalidation", "missing", "malformed", "inactive"} {
			t.Run(action+"/"+kind, func(t *testing.T) {
				v := recoveryResult(action)
				r := v["recovery"].(map[string]any)
				switch kind {
				case "version":
					v["responseVersion"] = "2"
				case "operation":
					v["operation"] = "reconcile"
				case "mode":
					v["mode"] = "PAUSED"
				case "status":
					v["status"] = "not-ready"
				case "attempt":
					v["checkAttemptId"] = "previous"
				case "configuration":
					v["configurationSha256"] = strings.Repeat("d", 64)
				case "release":
					v["releaseSha256"] = strings.Repeat("d", 64)
				case "generation":
					v["routingGeneration"] = "bad"
				case "prepared":
					v["baselinePrepared"] = true
				case "selected":
					v["baselineSelected"] = true
				case "scan":
					v["scanCompleted"] = false
				case "code", "cleanup":
					v[map[string]string{"code": "code", "cleanup": "cleanupCode"}[kind]] = "failed"
				case "diagnostic", "operator", "review", "savedNotices", "closureReview", "recoveryReview":
					v[kind] = map[string]any{}
				case "missing-processing":
					delete(v, "processing")
				case "processing-count":
					delete(v["processing"].(map[string]any), "attempts")
				case "processing-type":
					v["processing"].(map[string]any)["attempts"] = "0"
				case "processing-negative":
					v["processing"].(map[string]any)["attempts"] = -1
				case "report-count":
					delete(v["reporting"].(map[string]any), "reviewed")
				case "report-type":
					v["reporting"].(map[string]any)["known"] = "true"
				case "missing-reporting":
					delete(v, "reporting")
				case "report-error":
					v["reporting"].(map[string]any)["code"] = "failed"
				case "missing-telemetry":
					delete(v, "telemetry")
				case "metric-error":
					v["telemetry"].(map[string]any)["code"] = "failed"
				case "metric-rejected":
					v["telemetry"].(map[string]any)["accepted"] = false
				case "request":
					r["requestId"] = "previous"
				case "wrong-action":
					r["status"] = "CLOSED"
				case "ambiguous":
					r["ambiguous"] = true
				case "reason":
					r["reason"] = "stale_review"
				case "invalidation":
					r["invalidationReason"] = "changed"
				case "missing":
					delete(v, "recovery")
				}
				body, _ := json.Marshal(v)
				if kind == "malformed" {
					body = []byte("bad")
				}
				confirm := "true"
				if kind == "inactive" {
					confirm = "false"
				}
				for _, predicate := range []string{string(match[1]), strings.ReplaceAll(string(output[1]), "local.recovery_result", "jsondecode(self.content)")} {
					expr := strings.NewReplacer("self.content", strconv.Quote(string(body)), "local.recovery_attempt_id", `"12345678-1234-1234-1234-123456789abc"`, "sha256(module.configuration.configuration_json)", strconv.Quote(strings.Repeat("a", 64)), "local.release_sha256", strconv.Quote(strings.Repeat("b", 64)), "module.configuration.recovery_request_id", `"r1-request"`, "module.configuration.recovery_kind", strconv.Quote(action), "var.recovery_confirm", confirm, "var.mode", `"LIVE"`, "\n", " ").Replace(predicate)
					got, err := terraform(t, t.TempDir(), expr+"\n", "console", "-no-color")
					want := kind == "valid" || kind == "ambiguous"
					if err != nil || strings.TrimSpace(string(got)) != strconv.FormatBool(want) {
						t.Fatalf("predicate %s: %s (%v)", kind, got, err)
					}
				}
			})
		}
	}
}
