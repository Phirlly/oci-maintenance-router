package infra_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func startupIAMFixture() map[string]any {
	iam := map[string]any{
		"group": map[string]any{"id": "group-a", "matching_rule": "function-a", "description": "display only"},
	}
	for _, name := range []string{"router", "announcements", "diagnostic", "operator", "live"} {
		policy := map[string]any{"id": name + "-a", "statements": []string{"grant " + name}, "description": "display only"}
		if name == "router" || name == "announcements" {
			iam[name] = policy
		} else {
			iam[name] = []any{policy}
		}
	}
	return iam
}

func startupGeneration(t *testing.T, iam map[string]any) string {
	t.Helper()
	source, err := os.ReadFile("startup_wait.tf")
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(string(source), `resource "time_sleep" "iam_propagation"`)
	if len(parts) != 2 {
		t.Fatal("cannot isolate the actual IAM generation expression")
	}
	replacements := []string{"oci_identity_dynamic_group.router", "var.iam.group"}
	for _, name := range []string{"router", "announcements", "diagnostic", "operator", "live"} {
		replacements = append(replacements, "oci_identity_policy."+name, "var.iam."+name)
	}
	dir := t.TempDir()
	configuration := "variable \"iam\" { type = any }\n" + strings.NewReplacer(replacements...).Replace(parts[0])
	if err := os.WriteFile(filepath.Join(dir, "main.tf"), []byte(configuration), 0600); err != nil {
		t.Fatal(err)
	}
	writeJSON(t, filepath.Join(dir, "terraform.tfvars.json"), map[string]any{"iam": iam})
	output, err := terraform(t, dir, "local.startup_iam_generation\n", "console", "-no-color")
	var generation string
	if err != nil || json.Unmarshal(output, &generation) != nil || !regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(generation) {
		t.Fatalf("invalid IAM generation: %s (%v)", output, err)
	}
	return generation
}

func TestStartupGenerationTracksEveryEffectivePermissionChange(t *testing.T) {
	requireTerraform(t)
	baseline := startupGeneration(t, startupIAMFixture())
	for _, field := range []string{"id", "matching_rule"} {
		t.Run("group/"+field, func(t *testing.T) {
			iam := startupIAMFixture()
			iam["group"].(map[string]any)[field] = "changed"
			if startupGeneration(t, iam) == baseline {
				t.Fatal("changed group authority did not renew the wait")
			}
		})
	}
	for _, name := range []string{"router", "announcements", "diagnostic", "operator", "live"} {
		for _, field := range []string{"id", "statements"} {
			t.Run(name+"/"+field, func(t *testing.T) {
				iam := startupIAMFixture()
				var policy map[string]any
				if name == "router" || name == "announcements" {
					policy = iam[name].(map[string]any)
				} else {
					policy = iam[name].([]any)[0].(map[string]any)
				}
				if field == "statements" {
					policy[field] = []string{"changed grant"}
				} else {
					policy[field] = "replacement-policy"
				}
				if startupGeneration(t, iam) == baseline {
					t.Fatal("changed policy authority did not renew the wait")
				}
			})
		}
	}
	for _, name := range []string{"diagnostic", "operator", "live"} {
		t.Run(name+"/remove-and-restore", func(t *testing.T) {
			iam := startupIAMFixture()
			original := iam[name]
			iam[name] = []any{}
			if startupGeneration(t, iam) == baseline {
				t.Fatal("removed policy did not renew the wait")
			}
			iam[name] = original
			if startupGeneration(t, iam) != baseline {
				t.Fatal("identical restored permissions changed generation")
			}
		})
	}
}

func TestStartupGenerationIgnoresDisplayMetadataAndSupportsEmptyPolicies(t *testing.T) {
	requireTerraform(t)
	iam := startupIAMFixture()
	baseline := startupGeneration(t, iam)
	iam["group"].(map[string]any)["description"] = "new display text"
	iam["router"].(map[string]any)["description"] = "new policy display text"
	if startupGeneration(t, iam) != baseline || startupGeneration(t, iam) != baseline {
		t.Fatal("unchanged authority caused wait churn")
	}
	for _, name := range []string{"diagnostic", "operator", "live"} {
		iam[name] = []any{}
	}
	if startupGeneration(t, iam) == baseline {
		t.Fatal("empty optional policies did not change the generation")
	}
}
