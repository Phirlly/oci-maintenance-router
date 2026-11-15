package infra_test

import (
	"encoding/json"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

func receiptResult() map[string]any {
	return map[string]any{
		"responseVersion": 2, "status": "operational-verified", "operation": "deployment-check", "mode": "TEST", "checkAttemptId": "attempt",
		"configurationSha256": strings.Repeat("a", 64), "releaseSha256": strings.Repeat("b", 64), "routingGeneration": strings.Repeat("c", 64),
		"baselinePrepared": false, "baselineSelected": false, "scanCompleted": false,
		"diagnostic": map[string]any{"state": "API_ACCEPTED", "dispatched": false, "receiptVerified": true},
		"operator":   map[string]any{"state": "API_ACCEPTED", "dispatched": false, "receiptVerified": true},
	}
}

func receiptExpression(t *testing.T, expression string, result any, confirming, operator bool) string {
	t.Helper()
	body, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	quoted, _ := json.Marshal(string(body))
	return strings.NewReplacer("self.content", string(quoted), "var.mode", `"TEST"`, "local.operator_test_enabled", strconv.FormatBool(operator),
		"local.receipt_confirmation", strconv.FormatBool(confirming), "local.check_attempt_id", `"attempt"`,
		"sha256(module.configuration.configuration_json)", `"`+strings.Repeat("a", 64)+`"`, "local.release_sha256", `"`+strings.Repeat("b", 64)+`"`, "\n", " ").Replace(expression)
}

func TestConfirmationInvocationRequiresEveryProofWithoutDispatch(t *testing.T) {
	source, err := os.ReadFile("deployment_check.tf")
	if err != nil {
		t.Fatal(err)
	}
	match := regexp.MustCompile(`(?s)postcondition\s*\{\s*condition\s*=\s*(.*?)\s*error_message\s*=`).FindSubmatch(source)
	if len(match) != 2 {
		t.Fatal("actual invocation predicate missing")
	}
	for _, channel := range []string{"diagnostic", "operator"} {
		for _, kind := range []string{"valid", "missing", "false", "null", "string", "number", "dispatch", "string-dispatch", "null-dispatch", "null-status", "zero-status", "sent-status", "empty-error", "failed", "stale", "cleanup"} {
			t.Run(channel+"/"+kind, func(t *testing.T) {
				result := receiptResult()
				proof := result[channel].(map[string]any)
				switch kind {
				case "missing":
					delete(proof, "receiptVerified")
				case "false":
					proof["receiptVerified"] = false
				case "null":
					proof["receiptVerified"] = nil
				case "string":
					proof["receiptVerified"] = "true"
				case "number":
					proof["receiptVerified"] = 1
				case "dispatch":
					proof["dispatched"] = true
					proof["observedStatus"] = 202
				case "string-dispatch":
					proof["dispatched"] = "false"
				case "null-dispatch":
					proof["dispatched"] = nil
				case "null-status":
					proof["observedStatus"] = nil
				case "zero-status":
					proof["observedStatus"] = 0
				case "sent-status":
					proof["observedStatus"] = 202
				case "empty-error":
					proof["code"] = ""
				case "failed":
					result["status"] = "not-ready"
				case "stale":
					result["configurationSha256"] = strings.Repeat("d", 64)
				case "cleanup":
					result["cleanupCode"] = "release_failed"
				}
				expression := receiptExpression(t, string(match[1]), result, true, true)
				output, err := terraform(t, t.TempDir(), expression+"\n", "console", "-no-color")
				if err != nil || strings.TrimSpace(string(output)) != strconv.FormatBool(kind == "valid") {
					t.Fatalf("confirmation acceptance: %s (%v)", output, err)
				}
			})
		}
	}
}
