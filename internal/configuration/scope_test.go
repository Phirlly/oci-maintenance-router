package configuration

import (
	"reflect"
	"testing"
)

func TestRejectInvalidScope(t *testing.T) {
	cases := []struct {
		field string
		value any
	}{
		{"environmentName", ""}, {"environmentName", "-example"}, {"environmentName", "Example"},
		{"environmentName", "abcdefghijklmnopqrstuvwxyz1234567"}, {"environmentName", 1},
		{"tenancyId", "ocid1.compartment.oc1..example"}, {"tenancyId", "ocid1.tenancy.oc2..example"},
		{"tenancyId", "ocid1.tenancy.oc1.iad.example"}, {"tenancyId", "ocid1.tenancy.oc1.."},
		{"tenancyId", "ocid1.tenancy.oc1../example"}, {"tenancyId", "ocid1.tenancy.oc1..bad?x"},
		{"tenancyId", "ocid1.tenancy.oc1....example"},
		{"deploymentRegion", "US East (Ashburn)"}, {"deploymentRegion", "https://host"},
		{"regionMap", map[string]any{}}, {"regionMap", map[string]any{"us-ashburn-1": "phx"}},
		{"regionMap", map[string]any{"unknown": "iad"}}, {"regionMap", map[string]any{"us-ashburn-1": 1}},
		{"compartmentIds", []any{}}, {"compartmentIds", []any{nil}},
		{"compartmentIds", []any{"ocid1.compartment.oc1..example", "ocid1.compartment.oc1..example"}},
		{"compartmentIds", []any{"ocid1.tenancy.oc1..example"}},
		{"mysqlServices", []any{}}, {"mysqlServices", []any{"HeatWave on AWS"}},
		{"mysqlServices", []any{"Oracle HeatWave MySQL", "Oracle HeatWave MySQL"}},
		{"mysqlServices", "Oracle HeatWave MySQL"},
	}
	for _, tc := range cases {
		t.Run(tc.field, func(t *testing.T) {
			value := settings(t)
			value[tc.field] = tc.value
			_, err := Decode(encoded(t, value), catalog())
			detail, ok := err.(*Error)
			if !ok || detail.Field != tc.field {
				t.Fatalf("expected %s error, got %v", tc.field, err)
			}
		})
	}
}

func TestCatalogMustBeIndependentAndConsistent(t *testing.T) {
	for _, metadata := range []Catalog{
		{}, {Regions: map[string]string{"us-ashburn-1": "IAD"}, MySQLServices: []string{"Oracle HeatWave MySQL"}},
		{Regions: map[string]string{"us-ashburn-1": "iad", "us-phoenix-1": "iad"}, MySQLServices: []string{"Oracle HeatWave MySQL"}},
		{Regions: map[string]string{"bad/region": "iad"}, MySQLServices: []string{"Oracle HeatWave MySQL"}},
		{Regions: catalog().Regions, MySQLServices: []string{"", "Oracle HeatWave MySQL"}},
		{Regions: catalog().Regions, MySQLServices: []string{"Oracle HeatWave MySQL", "Oracle HeatWave MySQL"}},
	} {
		_, err := Decode(fixture(t), metadata)
		detail, ok := err.(*Error)
		if !ok || detail.Field != "catalog" {
			t.Fatalf("expected catalog error, got %v", err)
		}
	}
	metadata := catalog()
	before := catalog()
	got, err := Decode(fixture(t), metadata)
	if err != nil {
		t.Fatal(err)
	}
	got.RegionMap["us-ashburn-1"] = "changed"
	got.MySQLServices[0] = "changed"
	if !reflect.DeepEqual(metadata, before) {
		t.Fatal("decode mutated or aliased trusted catalog")
	}
}

func TestAcceptDocumentedOCIDVariants(t *testing.T) {
	value := settings(t)
	value["tenancyId"] = "ocid1.tenancy.oc1...example"
	value["compartmentIds"] = []any{"ocid1.compartment.oc1...example"}
	value["destinations"] = []any{map[string]any{"id": "payments", "secretId": "ocid1.vaultsecret.oc1.us-ashburn-1..example"}}
	value["intakeTopicId"] = "ocid1.onstopic.oc1.us-ashburn-1..intake"
	decoded(t, value)
}

func TestDeploymentRegionNeedNotBeMySQLScope(t *testing.T) {
	value := settings(t)
	value["regionMap"] = map[string]any{"us-phoenix-1": "phx"}
	decoded(t, value)
}
