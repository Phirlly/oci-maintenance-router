package infra_test

import (
	"encoding/json"
	"os"
	"regexp"
	"strings"
	"testing"
)

func TestActualInvocationPredicateRejectsStaleOrMalformedResults(t *testing.T) {
	requireTerraform(t)
	data, err := os.ReadFile("deployment_check.tf")
	if err != nil {
		t.Fatal(err)
	}
	match := regexp.MustCompile(`(?s)postcondition\s*\{\s*condition\s*=\s*(.*?)\s*error_message\s*=`).FindSubmatch(data)
	if len(match) != 2 {
		t.Fatal("cannot locate actual invocation postcondition")
	}
	expectedConfig := strings.Repeat("a", 64)
	expectedRelease := strings.Repeat("b", 64)
	attempt := "12345678-1234-1234-1234-123456789abc"
	for _, kind := range []string{"valid", "version", "operation", "mode", "status", "coalesced", "prepared", "selected", "scan", "typed", "attempt", "configuration", "release", "generation", "code", "cleanupCode", "diagnostic", "operator", "processing", "recovery", "missing", "legacy", "malformed"} {
		t.Run(kind, func(t *testing.T) {
			result := map[string]any{
				"responseVersion": 2, "status": "operational-verified", "operation": "deployment-check", "mode": "PREVIEW",
				"checkAttemptId": attempt, "configurationSha256": expectedConfig, "releaseSha256": expectedRelease,
				"routingGeneration": strings.Repeat("c", 64), "baselinePrepared": true, "baselineSelected": false, "scanCompleted": false,
			}
			switch kind {
			case "version":
				result["responseVersion"] = "2"
			case "operation":
				result["operation"] = "reconcile"
			case "mode":
				result["mode"] = "LIVE"
			case "status":
				result["status"] = "not-ready"
			case "coalesced":
				result["status"] = "coalesced"
			case "prepared":
				result["baselinePrepared"] = false
			case "selected":
				result["baselineSelected"] = true
			case "scan":
				result["scanCompleted"] = true
			case "typed":
				result["baselinePrepared"] = "true"
			case "attempt":
				result["checkAttemptId"] = "previous-attempt"
			case "configuration":
				result["configurationSha256"] = "previous-config"
			case "release":
				result["releaseSha256"] = "previous-release"
			case "generation":
				result["routingGeneration"] = "bad"
			case "code", "cleanupCode":
				result[kind] = "failed"
			case "diagnostic", "operator", "processing", "recovery":
				result[kind] = map[string]any{"dispatched": true}
			case "missing":
				delete(result, "baselinePrepared")
			case "legacy":
				result = map[string]any{"status": "deployment-verified", "sendingEnabled": false, "checkAttemptId": attempt, "configurationSha256": expectedConfig, "releaseSha256": expectedRelease, "routingGeneration": strings.Repeat("c", 64)}
			}
			encoded, err := json.Marshal(result)
			if err != nil {
				t.Fatal(err)
			}
			if kind == "malformed" {
				encoded = []byte("not JSON")
			}
			quote := func(s string) string {
				b, e := json.Marshal(s)
				if e != nil {
					t.Fatal(e)
				}
				return string(b)
			}
			expression := strings.NewReplacer(
				"var.mode", `"PREVIEW"`, "local.operator_test_enabled", "false",
				"self.content", quote(string(encoded)),
				"local.check_attempt_id", quote(attempt),
				"sha256(module.configuration.configuration_json)", quote(expectedConfig),
				"local.release_sha256", quote(expectedRelease),
				"\n", " ",
			).Replace(string(match[1]))
			output, err := terraform(t, t.TempDir(), expression+"\n", "console", "-no-color")
			want := "false"
			if kind == "valid" {
				want = "true"
			}
			if err != nil || strings.TrimSpace(string(output)) != want {
				t.Fatalf("%s predicate got %s (%v)", kind, output, err)
			}
		})
	}
}
