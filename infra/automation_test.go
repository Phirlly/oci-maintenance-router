package infra_test

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"testing"
)

func TestAutomaticScheduleUsesFixedBoundedWakeUp(t *testing.T) {
	source := readChecksumSource(t, "automation.tf")
	block := checksumBlock(source, `resource "oci_resource_scheduler_schedule" "reconcile"`)
	for _, pattern := range []string{`action\s*=\s*"START_RESOURCE"`, `recurrence_type\s*=\s*"CRON"`, `recurrence_details\s*=\s*"0 \* \* \* \*"`, `local_time_zone\s*=\s*"UTC"`, `compartment_id\s*=\s*var.compartment_ocid`, `id\s*=\s*oci_functions_function.router.id`, `parameter_type\s*=\s*"BODY"`, `depends_on\s*=\s*\[oci_functions_invoke_function.deployment_check\]`} {
		if !regexp.MustCompile(pattern).MatchString(block) {
			t.Fatal("missing schedule contract", pattern)
		}
	}
	if regexp.MustCompile(`(?m)^\s*(count|for_each|resource_filters)\s*[={]`).MatchString(block) {
		t.Fatal("schedule identity or target must remain static")
	}
	value := regexp.MustCompile(`(?m)^\s*value\s*=\s*(.+)$`).FindStringSubmatch(block)
	if len(value) != 2 {
		t.Fatal("missing explicit invocation body")
	}
	output, err := terraform(t, t.TempDir(), "jsonencode("+value[1]+")\n", "console", "-no-color")
	var encoded string
	var payloads []string
	if err != nil || json.Unmarshal(output, &encoded) != nil || json.Unmarshal([]byte(encoded), &payloads) != nil || len(payloads) != 1 {
		t.Fatalf("wrong provider BODY encoding %s: %v", output, err)
	}
	// OCI returns the BODY value as JSON text. The pinned provider decodes one
	// JSON layer on write, then encodes that API value again when reading it.
	var apiValue any
	if err := json.Unmarshal([]byte(payloads[0]), &apiValue); err != nil {
		t.Fatal("invalid provider BODY JSON", err)
	}
	if body, ok := apiValue.(string); !ok || body != `{"operation":"wake-up"}` {
		t.Fatalf("BODY must preserve the API's strict wake-up JSON text: %#v", apiValue)
	}
	roundTrip, err := json.Marshal(apiValue)
	if err != nil || string(roundTrip) != payloads[0] {
		t.Fatalf("BODY changes during the provider read/write round trip: %s (%v)", roundTrip, err)
	}
}

func TestAutomaticScheduleRemainsActiveAcrossModes(t *testing.T) {
	block := checksumBlock(readChecksumSource(t, "automation.tf"), `resource "oci_resource_scheduler_schedule" "reconcile"`)
	state := regexp.MustCompile(`(?m)^\s*state\s*=\s*(.+)$`).FindStringSubmatch(block)
	if len(state) != 2 {
		t.Fatal("missing explicit schedule state")
	}
	for _, mode := range []string{"PREVIEW", "TEST", "LIVE", "PAUSED"} {
		out, err := terraform(t, t.TempDir(), strings.ReplaceAll(state[1], "var.mode", fmt.Sprintf("%q", mode))+"\n", "console", "-no-color")
		if err != nil || strings.TrimSpace(string(out)) != `"ACTIVE"` {
			t.Error("schedule must stay active; runtime mode controls processing", mode, string(out), err)
		}
	}
}

func TestAnnouncementWakeUpUsesCatalogAndSeparateIntake(t *testing.T) {
	source := readChecksumSource(t, "automation.tf")
	announcement := checksumBlock(source, `resource "oci_announcements_service_announcement_subscription" "mysql"`)
	for _, pattern := range []string{`ons_topic_id\s*=\s*oci_ons_notification_topic.intake.id`, `for_each\s*=\s*toset\(local.catalog.mysqlServices\)`, `type\s*=\s*"SERVICE"`, `value\s*=\s*filters.value`, `depends_on\s*=\s*\[oci_ons_subscription.intake\]`} {
		if !regexp.MustCompile(pattern).MatchString(announcement) {
			t.Fatal("missing announcement contract", pattern)
		}
	}
	for _, forbidden := range []string{"RESOURCE_ID", "COMPARTMENT_ID", "ANNOUNCEMENT_TYPE", "var.mysql_compartments", "var.mysql_regions", "var.mode", "operator.id"} {
		if strings.Contains(announcement, forbidden) {
			t.Fatal("unstable/narrow announcement filter", forbidden)
		}
	}
	subscription := checksumBlock(source, `resource "oci_ons_subscription" "intake"`)
	for _, pattern := range []string{`topic_id\s*=\s*oci_ons_notification_topic.intake.id`, `protocol\s*=\s*"ORACLE_FUNCTIONS"`, `endpoint\s*=\s*oci_functions_function.router.id`, `depends_on\s*=\s*\[oci_functions_invoke_function.deployment_check\]`} {
		if !regexp.MustCompile(pattern).MatchString(subscription) {
			t.Fatal("missing subscription contract", pattern)
		}
	}
	if strings.Contains(subscription, "var.mode") || strings.Contains(subscription, "operator.id") {
		t.Fatal("subscription churn or feedback loop")
	}
}
