package delivery_test

import (
	"errors"
	"reflect"
	"testing"

	"oci-maintenance-router/internal/configuration"
	"oci-maintenance-router/internal/delivery"
	"oci-maintenance-router/internal/routing"
)

func TestBuilderRejectsInvalidConfigurationAndBootstrap(t *testing.T) {
	for _, tc := range []struct {
		name    string
		data    []byte
		catalog configuration.Catalog
		ledger  configuration.LedgerLocation
	}{
		{"json", []byte(`{"private-input":true}`), catalog(), ledger()},
		{"catalog", settings(t, nil), configuration.Catalog{}, ledger()},
		{"ledger", settings(t, nil), catalog(), configuration.LedgerLocation{}},
		{"aliases", settings(t, nil), configuration.Catalog{Regions: map[string]string{"us-ashburn-1": "iad", "iad": "other"}, MySQLServices: catalog().MySQLServices}, ledger()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b, err := delivery.New(tc.data, tc.catalog, tc.ledger)
			var detail *configuration.Error
			if b != nil || !errors.As(err, &detail) {
				t.Fatalf("uncontrolled/usable invalid builder: %v %v", b, err)
			}
		})
	}
}

func TestBuilderOwnsCustomerSettings(t *testing.T) {
	data, cat, location := settings(t, nil), catalog(), ledger()
	b, err := delivery.New(data, cat, location)
	if err != nil {
		t.Fatal(err)
	}
	for i := range data {
		data[i] = 'x'
	}
	cat.Regions["us-ashburn-1"], cat.MySQLServices[0] = "changed", "changed"
	location.Bucket = "changed"
	s := single(t, build(t, b, notice(t, record(db)), map[string]routing.Lookup{db: metadata(db, "payments")}, db))
	if s.EnvironmentID != "f8cffa22beb4df0f1675fa9016195e88248905435a8c65205e2916d8c6e2d8c0" || s.DestinationID != "payments" || s.SecretID != "ocid1.vaultsecret.oc1.iad.payments" || s.PagerDutyRegion != "US" {
		t.Fatalf("settings not retained: %+v", s)
	}
}

func TestDifferentCustomerNeedsNoCodeChanges(t *testing.T) {
	id := "ocid1.mysqldbsystem.oc1.phx.other"
	comp := "ocid1.compartment.oc1..other"
	data := settings(t, func(c map[string]any) {
		c["environmentName"], c["tenancyId"] = "another-environment", "ocid1.tenancy.oc1..other"
		c["deploymentRegion"], c["regionMap"] = "us-phoenix-1", map[string]string{"us-phoenix-1": "phx"}
		c["compartmentIds"], c["mysqlServices"] = []string{comp}, []string{"MySQL Database"}
		c["ownership"] = map[string]string{"kind": "freeform", "key": "Team"}
		c["destinations"] = []map[string]string{{"id": "dispatch-2", "secretId": "ocid1.vaultsecret.oc1.phx.other"}}
		c["routes"] = []map[string]string{{"ownerValue": "Team Ω", "destinationId": "dispatch-2"}}
		c["pagerDutyRegion"] = "EU"
		c["intakeTopicId"], c["operatorTopicId"] = "ocid1.onstopic.oc1.phx.intake", "ocid1.onstopic.oc1.phx.operator"
	})
	b, err := delivery.New(data, catalog(), configuration.LedgerLocation{Namespace: "another", Bucket: "ledger-2"})
	if err != nil {
		t.Fatal(err)
	}
	r := record(id)
	r.Services = []string{"MySQL Database"}
	lookup := routing.Lookup{State: routing.Found, Metadata: routing.Metadata{ID: &id, CompartmentID: &comp, FreeformTags: map[string]string{"Team": "Team Ω"}}}
	s := single(t, build(t, b, notice(t, r), map[string]routing.Lookup{id: lookup}, id))
	if s.DestinationID != "dispatch-2" || s.SecretID != "ocid1.vaultsecret.oc1.phx.other" || s.PagerDutyRegion != "EU" || !reflect.DeepEqual(s.Resources, []routing.Target{{ResourceID: id, Region: "us-phoenix-1"}}) {
		t.Fatalf("customer-specific value was assumed: %+v", s)
	}
	if envelope(t, s)["payload"].(map[string]any)["source"] != "oci-maintenance-router/another-environment" {
		t.Fatal("environment source was hardcoded")
	}
}
