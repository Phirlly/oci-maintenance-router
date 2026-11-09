package infra_test

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDestinationValidationExplainsCorrection(t *testing.T) {
	const validSecret = "ocid1.vaultsecret.oc1.iad.example"
	const names = "Destinations: add at least one entry. Names must use"
	const secrets = "Destinations: select a Vault secret in the stack's OCI region"
	cases := []struct {
		name         string
		destinations map[string]string
		want, absent string
	}{
		{"no destinations", map[string]string{}, names, secrets},
		{"invalid name", map[string]string{"payments": validSecret, "Invalid": validSecret}, names, secrets},
		{"name without selection", map[string]string{"payments": "payments"}, secrets, names},
		{"empty selection", map[string]string{"payments": ""}, secrets, names},
		{"wrong resource type", map[string]string{"payments": "ocid1.key.oc1.iad.example"}, secrets, names},
		{"foreign region", map[string]string{"payments": "ocid1.vaultsecret.oc1.phx.example"}, secrets, names},
		{"partially selected map", map[string]string{"payments": validSecret, "unselected-test": "unselected-test"}, secrets, names},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			settings := settingsFixture()
			settings["destinations"] = tc.destinations
			_, output, err := render(t, settings)
			message := strings.Join(strings.Fields(output), " ")
			if err == nil || !strings.Contains(message, tc.want) || strings.Contains(message, tc.absent) {
				t.Fatalf("want only destination correction %q, got %v: %s", tc.want, err, output)
			}
		})
	}
}

func TestDestinationValidationAcceptsCorrectedSelections(t *testing.T) {
	settings := settingsFixture()
	settings["destinations"] = map[string]string{
		"payments": "ocid1.vaultsecret.oc1.iad.example",
		"billing":  "ocid1.vaultsecret.oc1.us-ashburn-1..example",
	}
	got, output, err := render(t, settings)
	if err != nil {
		t.Fatalf("corrected selections rejected: %v: %s", err, output)
	}
	var config struct {
		Destinations []struct {
			ID       string `json:"id"`
			SecretID string `json:"secretId"`
		} `json:"destinations"`
	}
	if err := json.Unmarshal([]byte(got["configuration"]), &config); err != nil {
		t.Fatal(err)
	}
	want := settings["destinations"].(map[string]string)
	if len(config.Destinations) != len(want) {
		t.Fatalf("destination count changed: %d", len(config.Destinations))
	}
	for _, destination := range config.Destinations {
		if expected, ok := want[destination.ID]; !ok || destination.SecretID != expected {
			t.Fatalf("destination selection changed: %+v", destination)
		}
		delete(want, destination.ID)
	}
	if len(want) != 0 {
		t.Fatalf("destinations missing: %v", want)
	}
}
