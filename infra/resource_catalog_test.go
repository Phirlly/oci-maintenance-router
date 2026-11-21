package infra_test

import (
	"encoding/json"
	"regexp"
	"testing"
)

func TestReleaseCatalogValidationCoversEveryTypeAndBinding(t *testing.T) {
	block := checksumBlock(readChecksumSource(t, "automation.tf"), `resource "oci_announcements_service_announcement_subscription" "mysql"`)
	condition := regexp.MustCompile(`(?m)^\s*condition\s*=\s*(.+)$`).FindStringSubmatch(block)
	if len(condition) != 2 {
		t.Fatal("missing actual release validation")
	}
	for _, tc := range []struct {
		name   string
		change func(map[string]any)
	}{
		{"runtime-missing", func(r map[string]any) { delete(r["resourceServices"].(map[string]any), "instance") }},
		{"subscription-missing", func(r map[string]any) { delete(r["subscriptionServices"].(map[string]any), "instance") }},
		{"runtime-extra", func(r map[string]any) { r["resourceServices"].(map[string]any)["bucket"] = []string{"Storage"} }},
		{"subscription-extra", func(r map[string]any) { r["subscriptionServices"].(map[string]any)["bucket"] = []string{"Storage"} }},
		{"runtime-blank", func(r map[string]any) {
			r["resourceServices"].(map[string]any)["instance"] = []string{computeService, " "}
		}},
		{"runtime-duplicate", func(r map[string]any) {
			r["resourceServices"].(map[string]any)["instance"] = []string{computeService, computeService}
		}},
		{"runtime-empty", func(r map[string]any) { r["resourceServices"].(map[string]any)["instance"] = []string{} }},
		{"subscription-duplicate", func(r map[string]any) {
			r["subscriptionServices"].(map[string]any)["instance"] = []string{computeService, computeService}
		}},
		{"subscription-empty", func(r map[string]any) { r["subscriptionServices"].(map[string]any)["instance"] = []string{} }},
		{"cross-type", func(r map[string]any) {
			r["subscriptionServices"].(map[string]any)["instance"] = []string{currentMySQLService}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			release := releaseMetadata(t)
			tc.change(release)
			var valid bool
			if err := json.Unmarshal(serviceExpression(t, condition[1], release, "mysqldbsystem"), &valid); err != nil || valid {
				t.Fatalf("invalid independent binding accepted: %t (%v)", valid, err)
			}
		})
	}
}
