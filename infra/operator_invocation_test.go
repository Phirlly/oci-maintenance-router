package infra_test

import (
	"encoding/json"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

func TestOperatorInvocationRequiresMatchingAcceptedOutcome(t *testing.T) {
	requireTerraform(t)
	source, err := os.ReadFile("deployment_check.tf")
	if err != nil {
		t.Fatal(err)
	}
	match := regexp.MustCompile(`(?s)postcondition\s*\{\s*condition\s*=\s*(.*?)\s*error_message\s*=`).FindSubmatch(source)
	if len(match) != 2 {
		t.Fatal("missing actual postcondition")
	}
	for _, configured := range []bool{false, true} {
		for _, kind := range []string{"accepted", "reused", "missing", "pending", "error", "status", "string status", "string dispatch", "reused status", "null dispatch"} {
			t.Run(strconv.FormatBool(configured)+"/"+kind, func(t *testing.T) {
				operator := map[string]any{"state": "API_ACCEPTED", "dispatched": true, "observedStatus": 200}
				result := map[string]any{"responseVersion": 2, "status": "operational-verified", "operation": "deployment-check", "mode": "TEST",
					"checkAttemptId": "attempt", "configurationSha256": strings.Repeat("a", 64), "releaseSha256": strings.Repeat("b", 64), "routingGeneration": strings.Repeat("c", 64),
					"baselinePrepared": false, "baselineSelected": false, "scanCompleted": false,
					"diagnostic": map[string]any{"state": "API_ACCEPTED", "dispatched": false}, "operator": operator}
				switch kind {
				case "reused":
					operator["dispatched"] = false
					delete(operator, "observedStatus")
				case "missing":
					delete(result, "operator")
				case "pending":
					operator["state"] = "PUBLISHING"
				case "error":
					operator["code"] = "channel_read"
				case "status":
					operator["observedStatus"] = 202
				case "string status":
					operator["observedStatus"] = "200"
				case "string dispatch":
					operator["dispatched"] = "true"
				case "reused status":
					operator["dispatched"] = false
					operator["observedStatus"] = 0
				case "null dispatch":
					operator["dispatched"] = nil
				}
				body, _ := json.Marshal(result)
				quoted, _ := json.Marshal(string(body))
				expression := strings.NewReplacer("self.content", string(quoted), "var.mode", `"TEST"`, "local.operator_test_enabled", strconv.FormatBool(configured),
					"local.check_attempt_id", `"attempt"`, "sha256(module.configuration.configuration_json)", `"`+strings.Repeat("a", 64)+`"`, "local.release_sha256", `"`+strings.Repeat("b", 64)+`"`, "\n", " ").Replace(string(match[1]))
				output, err := terraform(t, t.TempDir(), expression+"\n", "console", "-no-color")
				want := configured && (kind == "accepted" || kind == "reused") || !configured && kind == "missing"
				if err != nil || strings.TrimSpace(string(output)) != strconv.FormatBool(want) {
					t.Fatalf("acceptance got %s (%v), want %v", output, err, want)
				}
			})
		}
	}
}
