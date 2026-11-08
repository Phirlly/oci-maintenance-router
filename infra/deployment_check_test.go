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
	for _, kind := range []string{"valid", "status", "sending", "attempt", "configuration", "release", "generation", "missing", "malformed"} {
		t.Run(kind, func(t *testing.T) {
			result := map[string]any{"status": "deployment-verified", "sendingEnabled": false, "checkAttemptId": attempt, "configurationSha256": expectedConfig, "releaseSha256": expectedRelease, "routingGeneration": strings.Repeat("c", 64)}
			switch kind {
			case "status":
				result["status"] = "not-ready"
			case "sending":
				result["sendingEnabled"] = "false"
			case "attempt":
				result["checkAttemptId"] = "previous-attempt"
			case "configuration":
				result["configurationSha256"] = "previous-config"
			case "release":
				result["releaseSha256"] = "previous-release"
			case "generation":
				result["routingGeneration"] = "bad"
			case "missing":
				delete(result, "sendingEnabled")
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
