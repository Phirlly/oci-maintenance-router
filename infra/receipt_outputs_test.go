package infra_test

import (
	"regexp"
	"strconv"
	"strings"
	"testing"
)

func TestReceiptOutputsNeverPromotePartialOrUnrequestedProof(t *testing.T) {
	source := readChecksumSource(t, "outputs.tf")
	match := regexp.MustCompile(`(?s)receipt_check_verified\s*=\s*(.*?)\n}`).FindStringSubmatch(source)
	if len(match) != 2 {
		t.Fatal("complete confirmation result predicate missing")
	}
	for _, kind := range []string{"both", "diagnostic-only", "partial", "failed", "stale", "configuration", "release", "cleanup", "unrequested", "malformed", "dispatch", "status-key", "false-proof", "null-proof", "string-proof", "unexpected-operator", "prepared", "selected", "scan", "recovery", "processing", "reporting"} {
		t.Run(kind, func(t *testing.T) {
			result := receiptResult()
			operator, confirming := true, true
			switch kind {
			case "diagnostic-only":
				delete(result, "operator")
				operator = false
			case "partial":
				delete(result["operator"].(map[string]any), "receiptVerified")
			case "failed":
				result["status"] = "not-ready"
			case "stale":
				result["checkAttemptId"] = "previous"
			case "configuration":
				result["configurationSha256"] = strings.Repeat("d", 64)
			case "release":
				result["releaseSha256"] = strings.Repeat("d", 64)
			case "cleanup":
				result["cleanupCode"] = "release_failed"
			case "unrequested":
				confirming = false
			case "malformed":
				result = map[string]any{}
			case "dispatch":
				result["diagnostic"].(map[string]any)["dispatched"] = true
			case "status-key":
				result["operator"].(map[string]any)["observedStatus"] = nil
			case "false-proof":
				result["diagnostic"].(map[string]any)["receiptVerified"] = false
			case "null-proof":
				result["diagnostic"].(map[string]any)["receiptVerified"] = nil
			case "string-proof":
				result["diagnostic"].(map[string]any)["receiptVerified"] = "true"
			case "unexpected-operator":
				operator = false
			case "prepared":
				result["baselinePrepared"] = true
			case "selected":
				result["baselineSelected"] = true
			case "scan":
				result["scanCompleted"] = true
			case "recovery", "processing", "reporting":
				result[kind] = map[string]any{}
			}
			expression := strings.ReplaceAll(match[1], "oci_functions_invoke_function.deployment_check.content", "self.content")
			expression = receiptExpression(t, expression, result, confirming, operator)
			for _, output := range []string{"test_receipt_confirmed", "operator_receipt_confirmed"} {
				block := checksumBlock(source, `output "`+output+`"`)
				value := regexp.MustCompile(`(?m)^\s*value\s*=\s*(.*)$`).FindStringSubmatch(block)
				if len(value) != 2 {
					t.Fatal("missing confirmation output", output)
				}
				bound := strings.NewReplacer("local.receipt_check_verified", "("+expression+")", "local.operator_test_enabled", strconv.FormatBool(operator)).Replace(value[1])
				got, err := terraform(t, t.TempDir(), bound+"\n", "console", "-no-color")
				want := (kind == "both" || kind == "diagnostic-only") && (output == "test_receipt_confirmed" || operator)
				if err != nil || strings.TrimSpace(string(got)) != strconv.FormatBool(want) {
					t.Fatalf("%s: %s (%v)", output, got, err)
				}
			}
		})
	}
}
