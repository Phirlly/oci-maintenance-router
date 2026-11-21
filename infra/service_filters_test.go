package infra_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"testing"
)

const currentMySQLService = "Oracle Cloud Infrastructure MySQL HeatWave"
const historicalMySQLService = "Oracle Cloud Infrastructure HeatWave MySQL"

func releaseMetadata(t *testing.T) map[string]any {
	t.Helper()
	var release map[string]any
	if err := json.Unmarshal([]byte(readChecksumSource(t, "release.json")), &release); err != nil {
		t.Fatal(err)
	}
	return release
}

func serviceExpression(t *testing.T, expression string, release map[string]any, selected ...string) []byte {
	t.Helper()
	requireTerraform(t)
	dir := t.TempDir()
	if len(selected) == 0 {
		selected = []string{"mysqldbsystem"}
	}
	writeJSON(t, filepath.Join(dir, "inputs.tf.json"), map[string]any{
		"locals": map[string]any{"release": release, "resource_type_ids": selected},
	})
	locals := regexp.MustCompile(`(?ms)^locals \{.*?^}`).FindString(readChecksumSource(t, "automation.tf"))
	if err := os.WriteFile(filepath.Join(dir, "main.tf"), []byte(locals), 0600); err != nil {
		t.Fatal(err)
	}
	out, err := terraform(t, dir, "jsonencode("+expression+")\n", "console", "-no-color")
	var encoded string
	if err != nil || json.Unmarshal(out, &encoded) != nil {
		t.Fatalf("service expression: %s (%v)", out, err)
	}
	return []byte(encoded)
}

func TestAnnouncementFiltersCoverOnlySelectedTypes(t *testing.T) {
	block := checksumBlock(readChecksumSource(t, "automation.tf"), `resource "oci_announcements_service_announcement_subscription" "mysql"`)
	expression := regexp.MustCompile(`(?m)^\s*for_each\s*=\s*(.+)$`).FindStringSubmatch(block)
	if len(expression) != 2 {
		t.Fatal("missing actual subscription filter expression")
	}
	for _, tc := range []struct{ types, want []string }{
		{[]string{"instance"}, []string{computeService}},
		{[]string{"mysqldbsystem", "instance"}, []string{computeService, currentMySQLService, "Oracle HeatWave MySQL"}},
		{[]string{"instance", "mysqldbsystem"}, []string{computeService, currentMySQLService, "Oracle HeatWave MySQL"}},
	} {
		var got []string
		if err := json.Unmarshal(serviceExpression(t, expression[1], releaseMetadata(t), tc.types...), &got); err != nil || !reflect.DeepEqual(got, tc.want) {
			t.Fatalf("selected filter union = %q (%v), want %q", got, err, tc.want)
		}
	}
}

func TestAnnouncementFiltersUseCurrentServiceNames(t *testing.T) {
	block := checksumBlock(readChecksumSource(t, "automation.tf"), `resource "oci_announcements_service_announcement_subscription" "mysql"`)
	expression := regexp.MustCompile(`(?m)^\s*for_each\s*=\s*(.+)$`).FindStringSubmatch(block)
	if len(expression) != 2 {
		t.Fatal("missing actual subscription filter expression")
	}
	var got []string
	if err := json.Unmarshal(serviceExpression(t, expression[1], releaseMetadata(t)), &got); err != nil {
		t.Fatal(err)
	}
	want := []string{currentMySQLService, "Oracle HeatWave MySQL"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("subscription filters = %q, want current API names %q", got, want)
	}
}

func TestAnnouncementFilterValidationRejectsInvalidReleaseLists(t *testing.T) {
	block := checksumBlock(readChecksumSource(t, "automation.tf"), `resource "oci_announcements_service_announcement_subscription" "mysql"`)
	condition := regexp.MustCompile(`(?m)^\s*condition\s*=\s*(.+)$`).FindStringSubmatch(block)
	if len(condition) != 2 {
		t.Fatal("missing subscription release-list validation")
	}
	for _, tc := range []struct {
		name  string
		list  []string
		valid bool
	}{
		{"release", nil, true},
		{"current", []string{currentMySQLService, "Oracle HeatWave MySQL"}, true},
		{"empty", []string{}, false},
		{"duplicate", []string{currentMySQLService, currentMySQLService}, false},
		{"unknown", []string{"Oracle HeatWave MySQL on AWS"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			release := releaseMetadata(t)
			if tc.list != nil {
				release["subscriptionServices"].(map[string]any)["mysqldbsystem"] = tc.list
			}
			var valid bool
			if err := json.Unmarshal(serviceExpression(t, condition[1], release), &valid); err != nil || valid != tc.valid {
				t.Fatalf("validation = %t (%v), want %t", valid, err, tc.valid)
			}
		})
	}
}
