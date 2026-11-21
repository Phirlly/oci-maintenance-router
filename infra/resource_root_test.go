package infra_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"oci-maintenance-router/internal/configuration"
	"oci-maintenance-router/internal/notices"
	"oci-maintenance-router/internal/routing"
)

func deploymentCatalog(t *testing.T) map[string]any {
	t.Helper()
	source := readChecksumSource(t, "providers.tf")
	start := strings.Index(source, "locals {")
	if start < 0 {
		t.Fatal("catalog locals missing")
	}
	source = strings.NewReplacer("data.oci_identity_region_subscriptions.tenancy.region_subscriptions", "var.regions", "var.environment_name", `"example"`, "var.compartment_ocid", `"ocid1.compartment.oc1..example"`, "var.region", `"us-ashburn-1"`).Replace(source[start:])
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.tf"), []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	writeJSON(t, filepath.Join(dir, "inputs.tf.json"), map[string]any{
		"locals": map[string]any{"release": releaseMetadata(t)}, "variable": map[string]any{"regions": map[string]any{"type": "any", "default": []map[string]any{
			{"region_name": "us-ashburn-1", "region_key": "IAD", "state": "READY", "is_home_region": true},
			{"region_name": "us-phoenix-1", "region_key": "PHX", "state": "READY", "is_home_region": false},
			{"region_name": "us-sanjose-1", "region_key": "SJC", "state": "IN_PROGRESS", "is_home_region": false},
		}}},
	})
	out, err := terraform(t, dir, "jsonencode(local.catalog)\n", "console", "-no-color")
	var raw string
	var catalog map[string]any
	if err != nil || json.Unmarshal(out, &raw) != nil || json.Unmarshal([]byte(raw), &catalog) != nil {
		t.Fatalf("root catalog: %s (%v)", out, err)
	}
	if !regexp.MustCompile(`resource_types\s*=\s*local.resource_type_ids`).MatchString(readChecksumSource(t, "configuration.tf")) {
		t.Fatal("root lost selection handoff")
	}
	return catalog
}

func TestRootConfigurationRoutesAllCategoriesForSelectedTypes(t *testing.T) {
	catalog := deploymentCatalog(t)
	var stableCatalog, environment string
	generations := map[string]bool{}
	for _, choices := range [][]string{{"HeatWave MySQL"}, {"Compute instances"}, {"HeatWave MySQL", "Compute instances"}} {
		t.Run(strings.Join(choices, "+"), func(t *testing.T) {
			types, message, err := rootResourceSelection(t, choices, false)
			if err != nil {
				t.Fatalf("selection: %s (%v)", message, err)
			}
			s := setupSettings("TEST", "SEND_TEST", true)
			s["resource_types"] = types
			values, message, err := renderWithCatalog(t, s, "b1-"+strings.Repeat("a", 64), catalog, "ocid1.onssubscription.oc1.iad.operator")
			if err != nil {
				t.Fatalf("root rendering: %s (%v)", message, err)
			}
			c := renderedConfig(t, values)
			trusted, err := configuration.DecodeCatalog([]byte(values["catalog"]))
			if err != nil {
				t.Fatal(err)
			}
			if len(trusted.Regions) != 2 || trusted.Regions["us-phoenix-1"] != "phx" {
				t.Fatal("independent READY region catalog changed")
			}
			if stableCatalog != "" && stableCatalog != values["catalog"] {
				t.Fatal("selection changed setup catalog bytes")
			}
			stableCatalog = values["catalog"]
			id, err := c.EnvironmentID(configuration.LedgerLocation{Namespace: "synthetic-namespace", Bucket: "synthetic-bucket"})
			if err != nil {
				t.Fatal(err)
			}
			if environment != "" && environment != id {
				t.Fatal("selection changed installation environment")
			}
			environment = id
			identities := setupIdentity(t, values, 1)
			if identities["environment"] != id {
				t.Fatal("setup IAM uses a different environment")
			}
			for _, channel := range []struct{ name, domain, prefix string }{{"diagnostic", "setup-diagnostic-v1", "sd1-"}, {"operator", "setup-operator-v1", "so1-"}} {
				encoded, err := json.Marshal([]any{channel.domain, id, c.InstallationID, 1})
				if err != nil {
					t.Fatal(err)
				}
				digest := sha256.Sum256(encoded)
				if identities[channel.name] != channel.prefix+hex.EncodeToString(digest[:]) {
					t.Fatal("selected setup IAM request identity differs", channel.name)
				}
			}
			generation, err := c.RoutingGeneration()
			if err != nil || generations[generation] {
				t.Fatal("selection did not change routing identity", err)
			}
			generations[generation] = true
			policy, err := routing.New([]byte(values["configuration"]), trusted)
			if err != nil {
				t.Fatal(err)
			}
			var resources []notices.Resource
			lookups := map[string]routing.Lookup{}
			want := []routing.Target{}
			for _, kind := range []string{"instance", "mysqldbsystem"} {
				resourceID, compartment := "ocid1."+kind+".oc1.phx.example", "ocid1.compartment.oc1..example"
				resources = append(resources, notices.Resource{ResourceID: resourceID})
				lookups[resourceID] = routing.Lookup{State: routing.Found, Metadata: routing.Metadata{ID: &resourceID, CompartmentID: &compartment, DefinedTags: map[string]map[string]any{"Operations": {"Owner": "payments"}}}}
				for _, selected := range types {
					if selected == kind {
						want = append(want, routing.Target{ResourceID: resourceID, Region: "us-phoenix-1"})
					}
				}
			}
			for _, category := range []string{"SCHEDULED_MAINTENANCE", "ACTION_REQUIRED", "PRODUCTION_EVENT_NOTIFICATION"} {
				platform := "IAAS"
				n, err := notices.Normalize(notices.Record{NoticeID: "ocid1.announcement.oc1..example", AnnouncementType: category, Platform: &platform, Services: []string{computeService, currentMySQLService}, Resources: resources})
				if err != nil {
					t.Fatal(err)
				}
				_, targets := policy.Targets(n, false)
				plan := policy.Resolve(n, false, lookups)
				if !reflect.DeepEqual(targets, want) || !reflect.DeepEqual(plan.Groups, []routing.Group{{DestinationID: "payments", Resources: want}}) {
					t.Fatalf("selected resource routing failed for %s: targets=%v groups=%v", category, targets, plan.Groups)
				}
			}
		})
	}
}
