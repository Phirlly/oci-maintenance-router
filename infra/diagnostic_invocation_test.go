package infra_test

import (
	"encoding/json"
	"os"
	"regexp"
	"strings"
	"testing"
)

func TestActualInvocationAcceptsOnlyCompletedConfiguredMode(t *testing.T) {
	requireTerraform(t)
	source, err := os.ReadFile("deployment_check.tf")
	if err != nil {
		t.Fatal(err)
	}
	match := regexp.MustCompile(`(?s)postcondition\s*\{\s*condition\s*=\s*(.*?)\s*error_message\s*=`).FindSubmatch(source)
	if len(match) != 2 {
		t.Fatal("actual postcondition missing")
	}
	for _, mode := range []string{"PAUSED", "TEST"} {
		for _, kind := range []string{"valid", "reused", "prepared", "selected", "scan", "mode", "error", "cleanup", "missing", "sending", "review", "dispatch-type", "status-type", "status", "diagnostic-code", "operator", "processing", "recovery"} {
			t.Run(mode+"/"+kind, func(t *testing.T) {
				diagnostic := map[string]any{"state": "API_ACCEPTED", "dispatched": true, "observedStatus": 202}
				result := map[string]any{"responseVersion": 2, "status": "operational-verified", "operation": "deployment-check", "mode": mode,
					"checkAttemptId": "attempt", "configurationSha256": strings.Repeat("a", 64), "releaseSha256": strings.Repeat("b", 64),
					"routingGeneration": strings.Repeat("c", 64), "baselinePrepared": false, "baselineSelected": false, "scanCompleted": false}
				if mode == "TEST" {
					result["diagnostic"] = diagnostic
				}
				switch kind {
				case "reused":
					diagnostic["dispatched"] = false
					delete(diagnostic, "observedStatus")
				case "prepared":
					result["baselinePrepared"] = true
				case "selected":
					result["baselineSelected"] = true
				case "scan":
					result["scanCompleted"] = true
				case "mode":
					result["mode"] = "PREVIEW"
				case "error":
					result["code"] = "failed"
				case "cleanup":
					result["cleanupCode"] = "failed"
				case "missing":
					delete(result, "diagnostic")
					delete(result, "baselinePrepared")
				case "sending", "review":
					diagnostic["state"] = strings.ToUpper(kind)
				case "dispatch-type":
					diagnostic["dispatched"] = "true"
				case "status-type":
					diagnostic["observedStatus"] = "202"
				case "status":
					diagnostic["observedStatus"] = 500
				case "diagnostic-code":
					diagnostic["code"] = "identity_conflict"
				case "operator", "processing", "recovery":
					result[kind] = map[string]any{}
				}
				// PAUSED must reject even a valid diagnostic; test-related mutations
				// insert one instead of merely modifying an absent result.
				if mode == "PAUSED" && (kind == "reused" || kind == "sending" || kind == "review" || strings.HasPrefix(kind, "dispatch") || strings.HasPrefix(kind, "status") || kind == "diagnostic-code") {
					result["diagnostic"] = diagnostic
				}
				body, _ := json.Marshal(result)
				quoted, _ := json.Marshal(string(body))
				expression := strings.NewReplacer("local.receipt_confirmation", "false", "self.content", string(quoted), "local.mode", `"`+mode+`"`, "local.operator_test_enabled", "false", "local.check_attempt_id", `"attempt"`,
					"sha256(module.configuration.configuration_json)", `"`+strings.Repeat("a", 64)+`"`, "local.release_sha256", `"`+strings.Repeat("b", 64)+`"`, "\n", " ").Replace(string(match[1]))
				output, err := terraform(t, t.TempDir(), expression+"\n", "console", "-no-color")
				want := "false"
				if kind == "valid" || mode == "TEST" && kind == "reused" {
					want = "true"
				}
				if err != nil || strings.TrimSpace(string(output)) != want {
					t.Fatalf("actual acceptance got %s (%v), want %s", output, err, want)
				}
			})
		}
	}
}
