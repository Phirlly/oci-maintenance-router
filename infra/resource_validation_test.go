package infra_test

import (
	"strings"
	"testing"
)

func TestSelectedConfigurationRejectsInvalidSelection(t *testing.T) {
	for _, choice := range [][]string{{}, {"instance", "instance"}, {"Instance"}, {"bucket"}, {""}} {
		s := setupSettings("PREVIEW", "NONE", false)
		s["resource_types"] = choice
		_, message, err := renderSelected(t, s)
		if err == nil || !strings.Contains(message, "Resource types") {
			t.Fatalf("invalid selection %q accepted or wrong error: %s (%v)", choice, message, err)
		}
	}
	s := settingsFixture()
	s["resource_types"] = []string{"instance"}
	_, message, err := renderSelected(t, s)
	if err == nil || !strings.Contains(message, "setup action") {
		t.Fatalf("selected legacy setup silently accepted: %s (%v)", message, err)
	}
}

func TestSelectedConfigurationRejectsIncompleteCatalog(t *testing.T) {
	for _, services := range []any{nil, map[string][]string{}, map[string][]string{"instance": {computeService}}, map[string][]string{"instance": {}, "mysqldbsystem": {currentMySQLService}}, map[string][]string{"instance": {computeService, computeService}, "mysqldbsystem": {currentMySQLService}}} {
		s := setupSettings("PREVIEW", "NONE", false)
		s["resource_types"] = []string{"instance"}
		catalog := selectedCatalog()
		catalog["resourceServices"] = services
		_, message, err := renderWithCatalog(t, s, "b1-"+strings.Repeat("a", 64), catalog)
		if err == nil || !strings.Contains(message, "Resource catalog") {
			t.Fatalf("invalid catalog accepted or wrong error: %s (%v)", message, err)
		}
	}
}
