package infra_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Real Terraform and the actual invoke body over synthetic retained state.
// terraform_data models body replacement; this is not a managed Function proof.
func TestClosureApplyKeepsActionIdentityAndRefreshesInvocation(t *testing.T) {
	requireTerraform(t)
	source, err := os.ReadFile("closure.tf")
	if err != nil {
		t.Fatal(err)
	}
	body := regexp.MustCompile(`(?s)invoke_function_body\s*=\s*(jsonencode\(\{.*?\}\))`).FindSubmatch(source)
	count := regexp.MustCompile(`(?m)^\s*count\s*=\s*(.*)$`).FindSubmatch(source)
	if len(body) != 2 || len(count) != 2 || !strings.Contains(string(source), "depends_on = [oci_functions_invoke_function.deployment_check]") {
		t.Fatal("missing explicit closure dependency/body/count")
	}
	dir := t.TempDir()
	module, err := filepath.Abs("modules/configuration")
	if err != nil {
		t.Fatal(err)
	}
	variables := map[string]any{"settings": map[string]string{"type": "any"}, "release": map[string]string{"type": "string"}}
	configuration := map[string]any{"source": module, "settings": "${var.settings}", "catalog": catalogFixture(),
		"installation_id": `${"b1-${sha256(terraform_data.bucket.output)}"}`,
		"topics":          map[string]string{"intake": "ocid1.onstopic.oc1.iad.intake", "operator": "ocid1.onstopic.oc1.iad.operator"}}
	invocation := "${" + string(body[1]) + "}"
	writeJSON(t, filepath.Join(dir, "main.tf.json"), map[string]any{
		"variable": variables, "module": map[string]any{"configuration": configuration},
		"locals": map[string]string{"closure_attempt_id": "${uuid()}", "release_sha256": "${var.release}"},
		"resource": map[string]any{"terraform_data": map[string]any{
			"bucket":     map[string]string{"input": "synthetic-retained-bucket"},
			"invocation": map[string]string{"count": "${" + strings.ReplaceAll(string(count[1]), "var.closure_confirm", "var.settings.closure_confirm") + "}", "input": invocation, "triggers_replace": invocation},
		}},
		"output": map[string]any{"invocations": map[string]string{"value": "${terraform_data.invocation[*].output}"}},
	})
	if out, err := terraform(t, dir, "", "init", "-backend=false", "-input=false", "-no-color"); err != nil {
		t.Fatalf("init: %s (%v)", out, err)
	}
	settings, release := closureSettings(), strings.Repeat("a", 64)
	first, previousAttempt := "", ""
	for _, step := range []string{"initial", "repeat", "release", "changed-code", "clear", "restore", "new-number"} {
		t.Run(step, func(t *testing.T) {
			switch step {
			case "release":
				release = strings.Repeat("b", 64)
			case "changed-code":
				intent := closureIntent()
				intent["targetId"] = strings.Repeat("d", 64)
				settings["closure_review_code"] = closureCode(intent)
			case "clear":
				settings["closure_confirm"] = false
			case "restore":
				settings["closure_confirm"] = true
			case "new-number":
				settings["closure_request_number"] = 8
			}
			writeJSON(t, filepath.Join(dir, "terraform.tfvars.json"), map[string]any{"settings": settings, "release": release})
			if out, err := terraform(t, dir, "", "apply", "-auto-approve", "-input=false", "-no-color"); err != nil {
				t.Fatalf("apply: %s (%v)", out, err)
			}
			out, err := terraform(t, dir, "", "output", "-json", "invocations")
			var values []string
			if err != nil || json.Unmarshal(out, &values) != nil {
				t.Fatalf("output: %s (%v)", out, err)
			}
			if step == "clear" {
				if len(values) != 0 {
					t.Fatal("clearing retained invocation")
				}
				return
			}
			if len(values) != 1 {
				t.Fatal("closure must have one invocation")
			}
			var request map[string]string
			if json.Unmarshal([]byte(values[0]), &request) != nil {
				t.Fatal("invalid actual invoke body")
			}
			if request["operation"] != "close-work" || request["releaseSha256"] != release || request["checkAttemptId"] == previousAttempt || request["checkAttemptId"] == "" {
				t.Fatal("stale or wrong invocation", request)
			}
			previousAttempt = request["checkAttemptId"]
			if first == "" {
				first = request["recoveryRequestId"]
			}
			if first == "" || (step == "new-number") == (request["recoveryRequestId"] == first) {
				t.Fatal("action identity changed implicitly or failed to change explicitly", request)
			}
		})
	}
}
