package infra_test

import (
	"regexp"
	"testing"
)

func TestFunctionLoggingIsProvisionedBeforeVerification(t *testing.T) {
	source := readChecksumSource(t, "logging.tf")
	for _, pattern := range []string{
		`resource "oci_logging_log_group" "router"`, `compartment_id\s*=\s*var.compartment_ocid`,
		`log_group_id\s*=\s*oci_logging_log_group.router.id`, `log_type\s*=\s*"SERVICE"`,
		`is_enabled\s*=\s*true`, `retention_duration\s*=\s*30`,
		`service\s*=\s*"functions"`, `resource\s*=\s*oci_functions_application.router.id`,
		`category\s*=\s*"invoke"`, `source_type\s*=\s*"OCISERVICE"`,
	} {
		if !regexp.MustCompile(pattern).MatchString(source) {
			t.Fatal("missing managed log contract", pattern)
		}
	}
	if !regexp.MustCompile(`logging\s*\{\s*line_format\s*=\s*"JSON"`).MatchString(readChecksumSource(t, "function.tf")) {
		t.Fatal("Function JSON logging missing")
	}
	if !regexp.MustCompile(`depends_on\s*=\s*\[[^]]*oci_logging_log.router`).MatchString(readChecksumSource(t, "deployment_check.tf")) {
		t.Fatal("deployment check can precede logging")
	}
}
