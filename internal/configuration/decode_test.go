package configuration

import (
	"reflect"
	"strings"
	"testing"
)

func TestDecodeDeploymentSettings(t *testing.T) {
	got, err := Decode(fixture(t), catalog())
	if err != nil {
		t.Fatal(err)
	}
	if got.SchemaVersion != 1 || got.EnvironmentName != "example" ||
		got.TenancyID != "ocid1.tenancy.oc1..example" || got.DeploymentRegion != "us-ashburn-1" ||
		got.PagerDutyRegion != "US" || got.Mode != "PREVIEW" || got.ActiveBaselineID != "" ||
		len(got.Replay) != 0 {
		t.Fatalf("incorrect settings/defaults: %+v", got)
	}
	if !reflect.DeepEqual(got.RegionMap, catalog().Regions) ||
		!reflect.DeepEqual(got.CompartmentIDs, []string{"ocid1.compartment.oc1..example"}) ||
		!reflect.DeepEqual(got.MySQLServices, []string{"Oracle HeatWave MySQL"}) ||
		got.Ownership != (Ownership{Kind: "defined", Namespace: "Operations", Key: "Owner"}) {
		t.Fatalf("incorrect scope/ownership: %+v", got)
	}
	if !reflect.DeepEqual(got.Routes, []Route{{"payments", "payments"}, {"billing", "payments"}}) ||
		!reflect.DeepEqual(got.Destinations, []Destination{{"payments", "ocid1.vaultsecret.oc1.iad.example"}}) ||
		got.IntakeTopicID != "ocid1.onstopic.oc1.iad.intake" ||
		got.OperatorTopicID != "ocid1.onstopic.oc1.iad.operator" {
		t.Fatalf("incorrect routing: %+v", got)
	}
}

func TestRequiredAndExactTopLevelFields(t *testing.T) {
	required := []string{"schemaVersion", "environmentName", "tenancyId", "deploymentRegion",
		"regionMap", "compartmentIds", "mysqlServices", "ownership", "destinations",
		"routes", "pagerDutyRegion", "intakeTopicId", "operatorTopicId"}
	for _, field := range required {
		t.Run(field, func(t *testing.T) {
			value := settings(t)
			delete(value, field)
			wantError(t, encoded(t, value), field, "required")
			value = settings(t)
			value[field] = nil
			wantError(t, encoded(t, value), field, "type")
		})
	}
	for _, name := range []string{"EnvironmentName", "limits", "routingKey", "untrusted-secret-name"} {
		t.Run(name, func(t *testing.T) {
			value := settings(t)
			value[name] = "sensitive"
			wantError(t, encoded(t, value), "configuration", "unknown_field")
		})
	}
}

func TestSchemaVersionIsExactInteger(t *testing.T) {
	for _, value := range []string{"9", "1.0", "1e0", "2.0", "2e0", "3.0", "3e0", "4.0", "4e0", "5.0", "5e0", "\"1\"", "\"2\"", "\"3\"", "\"4\"", "\"5\"", "true", "[]", "{}"} {
		t.Run(value, func(t *testing.T) {
			data := strings.Replace(string(fixture(t)), "\"schemaVersion\": 1", "\"schemaVersion\": "+value, 1)
			wantError(t, []byte(data), "schemaVersion", "unsupported")
		})
	}
}

func TestDecodePreservesExactOwnerStrings(t *testing.T) {
	value := settings(t)
	value["ownership"] = map[string]any{"kind": "freeform", "key": "Owner!"}
	value["routes"] = []any{map[string]any{"ownerValue": " Payments 🐬 ", "destinationId": "payments"}}
	value["pagerDutyRegion"] = "EU"
	value["activeBaselineId"] = nil
	got := decoded(t, value)
	if got.Ownership != (Ownership{Kind: "freeform", Key: "Owner!"}) ||
		got.Routes[0].OwnerValue != " Payments 🐬 " || got.PagerDutyRegion != "EU" {
		t.Fatalf("strings coerced: %+v", got)
	}
}
