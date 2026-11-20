package infra_test

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

func TestDeploymentCheckWaitsForIAMPropagation(t *testing.T) {
	source, err := os.ReadFile("deployment_check.tf")
	if err != nil {
		t.Fatal(err)
	}
	dependencies := regexp.MustCompile(`depends_on\s*=\s*\[([^]]*)\]`).FindSubmatch(source)
	if len(dependencies) != 2 || !strings.Contains(string(dependencies[1]), "time_sleep.iam_propagation") {
		t.Fatal("deployment check must follow the IAM propagation wait")
	}
}

func TestStartupWaitIsBoundedAndHasNoDestroyDelay(t *testing.T) {
	source, err := os.ReadFile("startup_wait.tf")
	if err != nil {
		t.Fatal(err)
	}
	block := checksumBlock(string(source), `resource "time_sleep" "iam_propagation"`)
	if !regexp.MustCompile(`create_duration\s*=\s*"5m"`).MatchString(block) {
		t.Fatal("the provisional startup wait must be five minutes")
	}
	if strings.Contains(block, "destroy_duration") || strings.Contains(block, "provisioner") {
		t.Fatal("startup wait must not delay destruction or execute a provisioner")
	}
	if !regexp.MustCompile(`triggers\s*=\s*\{\s*iam_generation\s*=\s*local.startup_iam_generation\s*\}`).MatchString(block) {
		t.Fatal("wait replacement must bind only the effective IAM generation")
	}
	dependencies := regexp.MustCompile(`depends_on\s*=\s*\[([^]]*)\]`).FindStringSubmatch(block)
	if len(dependencies) != 2 {
		t.Fatal("startup wait must follow every runtime policy transition")
	}
	for _, name := range []string{"router", "announcements", "diagnostic", "operator", "setup", "live"} {
		if !strings.Contains(dependencies[1], "oci_identity_policy."+name) {
			t.Fatalf("startup wait can precede the %s policy", name)
		}
	}
	versions, err := os.ReadFile("versions.tf")
	if err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`(?s)time\s*=\s*\{\s*source\s*=\s*"hashicorp/time"\s*version\s*=\s*"= 0\.14\.2"\s*\}`).Match(versions) {
		t.Fatal("startup provider must use the verified exact version")
	}
}
