package infra_test

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

func liveResult() map[string]any {
	r := receiptResult()
	r["mode"], r["baselineSelected"] = "LIVE", true
	delete(r, "diagnostic")
	delete(r, "operator")
	return r
}

func liveExpression(t *testing.T, expression string, result any, mode string) string {
	t.Helper()
	body, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	return strings.NewReplacer("oci_functions_invoke_function.deployment_check.content", strconv.Quote(string(body)), "self.content", strconv.Quote(string(body)),
		"var.mode", strconv.Quote(mode), "local.operator_test_enabled", "false", "local.receipt_confirmation", "false", "local.check_attempt_id", `"attempt"`,
		"sha256(module.configuration.configuration_json)", strconv.Quote(strings.Repeat("a", 64)), "local.release_sha256", strconv.Quote(strings.Repeat("b", 64)), "\n", " ").Replace(expression)
}

func TestLiveInvocationAndOutputRequireExactNonSendingSelection(t *testing.T) {
	invocation := readChecksumSource(t, "deployment_check.tf")
	match := regexp.MustCompile(`(?s)postcondition\s*\{\s*condition\s*=\s*(.*?)\s*error_message\s*=`).FindStringSubmatch(invocation)
	if len(match) != 2 {
		t.Fatal("missing invocation predicate")
	}
	activation := readChecksumSource(t, "activation.tf")
	output := regexp.MustCompile(`(?s)live_check_verified\s*=\s*(.*?)\n}`).FindStringSubmatch(activation)
	if len(output) != 2 {
		t.Fatal("missing LIVE result predicate")
	}
	if !regexp.MustCompile(`value\s*=\s*local.live_check_verified`).MatchString(checksumBlock(readChecksumSource(t, "outputs.tf"), `output "sending_enabled"`)) {
		t.Fatal("sending output must use complete verified result")
	}
	for _, kind := range []string{"valid", "missing", "unselected", "string-selection", "null-selection", "prepared", "scan", "version", "status", "operation", "mode", "attempt", "configuration", "release", "generation", "error", "cleanup", "diagnostic", "operator", "processing", "reporting", "telemetry", "recovery", "recoveryReview", "paused"} {
		t.Run(kind, func(t *testing.T) {
			r, mode := liveResult(), "LIVE"
			switch kind {
			case "missing":
				delete(r, "baselineSelected")
			case "unselected":
				r["baselineSelected"] = false
			case "string-selection":
				r["baselineSelected"] = "true"
			case "null-selection":
				r["baselineSelected"] = nil
			case "prepared":
				r["baselinePrepared"] = true
			case "scan":
				r["scanCompleted"] = true
			case "version":
				r["responseVersion"] = 1
			case "status":
				r["status"] = "not-ready"
			case "operation":
				r["operation"] = "reconcile"
			case "mode":
				r["mode"] = "TEST"
			case "attempt":
				r["checkAttemptId"] = "old"
			case "configuration":
				r["configurationSha256"] = strings.Repeat("d", 64)
			case "release":
				r["releaseSha256"] = strings.Repeat("d", 64)
			case "generation":
				r["routingGeneration"] = "bad"
			case "error":
				r["code"] = "failed"
			case "cleanup":
				r["cleanupCode"] = "release_failed"
			case "diagnostic", "operator", "processing", "reporting", "telemetry", "recovery", "recoveryReview":
				r[kind] = map[string]any{}
			case "paused":
				mode = "PAUSED"
			}
			for _, expression := range []string{match[1], output[1]} {
				got, err := terraform(t, t.TempDir(), liveExpression(t, expression, r, mode)+"\n", "console", "-no-color")
				if err != nil || strings.TrimSpace(string(got)) != strconv.FormatBool(kind == "valid") {
					t.Fatalf("result acceptance: %s (%v)", got, err)
				}
			}
		})
	}
}
