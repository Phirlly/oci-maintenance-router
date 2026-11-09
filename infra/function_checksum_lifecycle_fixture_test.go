package infra_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Substitute local Terraform resources for OCI while retaining the actual
// checksum condition, its owner, and the Function/verification/invocation edges.
type checksumLifecycle struct {
	t   *testing.T
	dir string
}

func newChecksumLifecycle(t *testing.T) checksumLifecycle {
	t.Helper()
	requireTerraform(t)
	f := checksumLifecycle{t: t, dir: t.TempDir()}
	source := readChecksumSource(t, "function.tf")
	match := regexp.MustCompile(`(?s)postcondition\s*\{\s*condition\s*=\s*(.*?)\s*error_message\s*=`).FindStringSubmatch(source)
	if len(match) != 2 {
		t.Fatal("cannot locate actual Function checksum postcondition")
	}
	condition := match[1]
	managedCheck, readCheck, readDependencies := "", "", ""
	if strings.Contains(checksumBlock(source, `resource "oci_functions_function" "router"`), "postcondition") {
		// Retain the original placement so the regression reproduces the stale
		// computed digest rejecting a changed archive during Plan.
		managedCheck = checksumCondition(strings.ReplaceAll(condition, "self.source_details", "self.input.source_details"))
	} else {
		block := checksumBlock(source, `data "oci_functions_function" "archive"`)
		if !regexp.MustCompile(`function_id\s*=\s*oci_functions_function\.router\.id`).MatchString(block) {
			t.Fatal("checksum read must select the managed Function")
		}
		readCheck = checksumCondition(strings.ReplaceAll(condition, "self.source_details", "self.outputs.source_details"))
		readDependencies = checksumDependencies(block)
	}
	invocation := checksumBlock(readChecksumSource(t, "deployment_check.tf"), `resource "oci_functions_invoke_function" "deployment_check"`)
	if !regexp.MustCompile(`function_id\s*=\s*oci_functions_function\.router\.id`).MatchString(invocation) {
		t.Fatal("diagnostic must invoke the managed Function")
	}
	invocationDependencies := checksumDependencies(invocation)
	if readCheck != "" && !strings.Contains(invocationDependencies, "data.terraform_remote_state.archive") {
		t.Fatal("diagnostic must depend on successful archive verification")
	}
	configuration := fmt.Sprintf(`
variable "desired" { type = string }
variable "reported" { type = string }
locals { archive_path = "${path.module}/function.zip" }
resource "terraform_data" "function" {
  input = {
    archive = var.desired
    source_details = [{source_code_sha256 = var.reported}]
  }
  %s
}
data "terraform_remote_state" "archive" {
  backend = "local"
  config = {path = "${path.module}/reported.tfstate"}
  defaults = {function_id = terraform_data.function.id}
  %s
  %s
}
resource "terraform_data" "diagnostic" {
  input = {archive = var.desired, function_id = terraform_data.function.id}
  triggers_replace = var.desired
  %s
}
`, managedCheck, readDependencies, readCheck, invocationDependencies)
	if err := os.WriteFile(filepath.Join(f.dir, "main.tf"), []byte(configuration), 0600); err != nil {
		t.Fatal(err)
	}
	f.run("init", "-backend=false", "-input=false", "-no-color")
	return f
}

func readChecksumSource(t *testing.T, name string) string {
	t.Helper()
	source, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	return string(source)
}

func checksumBlock(source, header string) string {
	start := strings.Index(source, header)
	if start < 0 {
		return ""
	}
	block := source[start:]
	if next := regexp.MustCompile(`(?m)^(resource|data) "`).FindStringIndex(block[len(header):]); next != nil {
		block = block[:len(header)+next[0]]
	}
	return block
}

func checksumCondition(condition string) string {
	return "lifecycle {\npostcondition {\ncondition = " + condition + "\nerror_message = \"OCI did not report the bundled Function archive checksum.\"\n}\n}"
}

func checksumDependencies(block string) string {
	match := regexp.MustCompile(`depends_on\s*=\s*\[([^]]*)\]`).FindStringSubmatch(block)
	if len(match) != 2 {
		return ""
	}
	var edges []string
	for _, edge := range strings.Split(match[1], ",") {
		switch strings.TrimSpace(edge) {
		case "oci_functions_function.router":
			edges = append(edges, "terraform_data.function")
		case "data.oci_functions_function.archive":
			edges = append(edges, "data.terraform_remote_state.archive")
		}
	}
	// IAM is outside this checksum-ordering fixture.
	return "depends_on = [" + strings.Join(edges, ",") + "]"
}

func (f checksumLifecycle) selectArchive(contents, reported string) string {
	f.t.Helper()
	archive := []byte(contents)
	digest := sha256.Sum256(archive)
	desired := hex.EncodeToString(digest[:])
	if err := os.WriteFile(filepath.Join(f.dir, "function.zip"), archive, 0600); err != nil {
		f.t.Fatal(err)
	}
	if reported == "" {
		reported = desired
	}
	writeJSON(f.t, filepath.Join(f.dir, "terraform.tfvars.json"), map[string]string{"desired": desired, "reported": reported})
	return desired
}

func (f checksumLifecycle) report(digest string) {
	f.t.Helper()
	details := []map[string]string{}
	if digest != "" {
		details = append(details, map[string]string{"source_code_sha256": digest})
	}
	writeJSON(f.t, filepath.Join(f.dir, "reported.tfstate"), map[string]any{
		"version": 4, "terraform_version": "1.5.7", "serial": 1,
		"lineage": "c8484d14-42bf-4707-ab17-5a307d1d9472", "resources": []any{},
		"outputs": map[string]any{"source_details": map[string]any{
			"type": []any{"list", []any{"object", map[string]string{"source_code_sha256": "string"}}}, "value": details,
		}},
	})
}

func (f checksumLifecycle) run(args ...string) []byte {
	f.t.Helper()
	output, err := terraform(f.t, f.dir, "", args...)
	if err != nil {
		f.t.Fatalf("terraform %v: %s (%v)", args, output, err)
	}
	return output
}

func (f checksumLifecycle) plan() map[string][]string {
	f.t.Helper()
	f.run("plan", "-input=false", "-no-color", "-out=change.tfplan")
	var plan struct {
		Changes []struct {
			Address string
			Change  struct{ Actions []string }
		} `json:"resource_changes"`
	}
	if err := json.Unmarshal(f.run("show", "-json", "change.tfplan"), &plan); err != nil {
		f.t.Fatal(err)
	}
	actions := map[string][]string{}
	for _, change := range plan.Changes {
		actions[change.Address] = change.Change.Actions
	}
	return actions
}

func (f checksumLifecycle) diagnosticArchive() string {
	f.t.Helper()
	var state struct {
		Values struct {
			Root struct {
				Resources []struct {
					Address string
					Values  struct{ Input struct{ Archive string } }
				}
			} `json:"root_module"`
		}
	}
	if err := json.Unmarshal(f.run("show", "-json"), &state); err != nil {
		f.t.Fatal(err)
	}
	for _, resource := range state.Values.Root.Resources {
		if resource.Address == "terraform_data.diagnostic" {
			return resource.Values.Input.Archive
		}
	}
	return ""
}
