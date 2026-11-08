package infra_test

import (
	"encoding/json"
	"testing"

	"oci-maintenance-router/internal/configuration"
)

func TestRenderedConfigurationMatchesRuntimeContract(t *testing.T) {
	got, message, err := render(t, settingsFixture())
	if err != nil {
		t.Fatalf("render: %s %v", message, err)
	}
	var catalog configuration.Catalog
	if err := json.Unmarshal([]byte(got["catalog"]), &catalog); err != nil {
		t.Fatal(err)
	}
	config, err := configuration.Decode([]byte(got["configuration"]), catalog)
	if err != nil {
		t.Fatalf("generated configuration rejected by runtime: %v", err)
	}
	if config.Mode != "PREVIEW" || config.ActiveBaselineID != "" || len(config.Replay) != 0 {
		t.Fatal("installation configuration can activate delivery")
	}
	if config.RegionMap["us-phoenix-1"] != "phx" || len(config.RegionMap) != 1 || len(catalog.Regions) != 2 {
		t.Fatal("scope replaced independent catalog")
	}
	if len(config.Routes) != 2 || len(config.Destinations) != 1 || config.Ownership.Namespace != "Operations" {
		t.Fatal("routing configuration lost customer settings")
	}
}
