package infra_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
)

func TestMonitoringQueriesUseStableScopeAndObservedSignals(t *testing.T) {
	source := readChecksumSource(t, "monitoring.tf")
	match := regexp.MustCompile(`(?s)operational_alarms\s*=\s*(\{.*\})\s*\}\s*resource`).FindStringSubmatch(source)
	if len(match) != 2 {
		t.Fatal("missing alarm definitions")
	}
	expression := strings.NewReplacer("local.installation_id", `"installation-1"`, "oci_functions_function.router.id", `"ocid1.fnfunc.oc1.iad.router"`).Replace(match[1])
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.tf"), []byte("locals {\n alarms = "+expression+"\n}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	output, err := terraform(t, dir, "jsonencode(local.alarms)\n", "console", "-no-color")
	var encoded string
	var values map[string]struct{ Namespace, Query, Body, Severity string }
	if err != nil || json.Unmarshal(output, &encoded) != nil || json.Unmarshal([]byte(encoded), &values) != nil {
		t.Fatalf("alarm rendering: %s %v", output, err)
	}
	want := map[string]string{
		"missing-scan":   `CompletedScan[2h]{installationId="installation-1"}.groupBy(installationId).absent(72h)`,
		"review-work":    `ReviewIssues[2h]{installationId="installation-1"}.groupBy(installationId).last() > 0`,
		"report-failure": `OperatorReportFailed[2h]{installationId="installation-1"}.groupBy(installationId).last() > 0`,
		"function-error": `FunctionResponseCount[5m]{resourceId="ocid1.fnfunc.oc1.iad.router",responseType="Error"}.grouping().sum() > 0`,
	}
	got := map[string]string{}
	for name, value := range values {
		got[name] = value.Query
		namespace := "maintenance_router"
		if name == "function-error" {
			namespace = "oci_faas"
		}
		if value.Namespace != namespace || value.Body == "" || value.Severity != "ERROR" {
			t.Fatal("missing alarm namespace/action/severity", name)
		}
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected alarm queries: %v", got)
	}
	if !strings.Contains(values["missing-scan"].Body, "RESET") || !strings.Contains(values["missing-scan"].Body, "first") {
		t.Fatal("absence alarm hides startup/reset limitation")
	}
}

func TestMonitoringIsStableAndEnabledOnlyInLiveMode(t *testing.T) {
	source := readChecksumSource(t, "monitoring.tf")
	for _, pattern := range []string{
		`for_each\s*=\s*local.operational_alarms`, `compartment_id\s*=\s*var.compartment_ocid`,
		`metric_compartment_id\s*=\s*var.compartment_ocid`, `metric_compartment_id_in_subtree\s*=\s*false`,
		`destinations\s*=\s*\[oci_ons_notification_topic.operator.id\]`, `message_format\s*=\s*"ONS_OPTIMIZED"`,
		`resolution\s*=\s*"1m"`, `pending_duration\s*=\s*"PT1M"`, `evaluation_slack_duration\s*=\s*"PT3M"`, `repeat_notification_duration\s*=\s*"PT2H"`,
		`namespace\s*=\s*each.value.namespace`, `query\s*=\s*each.value.query`, `body\s*=\s*each.value.body`,
	} {
		if !regexp.MustCompile(pattern).MatchString(source) {
			t.Fatal("missing alarm contract", pattern)
		}
	}
	if regexp.MustCompile(`(?m)^\s*(count|suppression|resource_group)\s*[={]`).MatchString(source) {
		t.Fatal("alarm lifecycle or grouping changed")
	}
	enabled := regexp.MustCompile(`(?m)^\s*is_enabled\s*=\s*(.*)$`).FindStringSubmatch(source)
	if len(enabled) != 2 {
		t.Fatal("missing mode guard")
	}
	for _, mode := range []string{"PREVIEW", "PAUSED", "TEST", "LIVE"} {
		output, err := terraform(t, t.TempDir(), strings.ReplaceAll(enabled[1], "local.mode", fmt.Sprintf("%q", mode))+"\n", "console", "-no-color")
		if err != nil || strings.TrimSpace(string(output)) != fmt.Sprint(mode == "LIVE") {
			t.Fatal("alarm mode guard failed", mode)
		}
	}
	if !regexp.MustCompile(`depends_on\s*=\s*\[[^]]*oci_monitoring_alarm.router`).MatchString(readChecksumSource(t, "deployment_check.tf")) {
		t.Fatal("activation can precede alarm creation")
	}
	if !regexp.MustCompile(`metricsCompartmentId\s*=\s*var.compartment_ocid`).MatchString(readChecksumSource(t, "function.tf")) {
		t.Fatal("metrics lack deployment-owned compartment")
	}
}
