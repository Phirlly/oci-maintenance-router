package infra_test

import (
	"regexp"
	"strconv"
	"strings"
	"testing"
)

func TestReceiptInvocationSeparatesConfirmationFromSubmission(t *testing.T) {
	source := readChecksumSource(t, "deployment_check.tf")
	match := regexp.MustCompile(`(?s)postcondition\s*\{\s*condition\s*=\s*(.*?)\s*error_message\s*=`).FindStringSubmatch(source)
	if len(match) != 2 {
		t.Fatal("actual invocation predicate missing")
	}
	for _, kind := range []string{"diagnostic-only", "missing-operator", "unexpected-operator", "malformed", "send", "reuse", "unrequested-true", "unrequested-false", "unrequested-null"} {
		t.Run(kind, func(t *testing.T) {
			result := receiptResult()
			operator, confirming, want := true, true, false
			switch kind {
			case "diagnostic-only":
				delete(result, "operator")
				operator, want = false, true
			case "missing-operator":
				delete(result, "operator")
			case "unexpected-operator":
				operator = false
			case "malformed":
				result = map[string]any{}
			default:
				confirming = false
				for _, channel := range []string{"diagnostic", "operator"} {
					proof := result[channel].(map[string]any)
					delete(proof, "receiptVerified")
					if kind == "send" {
						proof["dispatched"], proof["observedStatus"] = true, 202
					}
				}
				if kind == "send" || kind == "reuse" {
					want = true
				} else {
					var marker any
					if kind == "unrequested-true" {
						marker = true
					} else if kind == "unrequested-false" {
						marker = false
					}
					result["operator"].(map[string]any)["receiptVerified"] = marker
				}
			}
			expression := receiptExpression(t, match[1], result, confirming, operator)
			out, err := terraform(t, t.TempDir(), expression+"\n", "console", "-no-color")
			if err != nil || strings.TrimSpace(string(out)) != strconv.FormatBool(want) {
				t.Fatalf("mode separation: %s (%v)", out, err)
			}
		})
	}
}
