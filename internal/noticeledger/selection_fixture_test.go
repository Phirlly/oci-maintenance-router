package noticeledger

import (
	"testing"

	"oci-maintenance-router/internal/configuration"
)

const selectedInstance = "ocid1.instance.oc1.iad.selected"
const selectedComputeService = "Oracle Cloud Infrastructure Compute"

func selectedObserver(t *testing.T, kinds ...string) *Observer {
	t.Helper()
	cat := configuration.Catalog{SchemaVersion: 1, Regions: catalog().Regions,
		ResourceServices: map[string][]string{"mysqldbsystem": catalog().MySQLServices, "instance": {selectedComputeService}}}
	data := settings(func(v map[string]any) {
		v["schemaVersion"], v["resourceTypes"] = 9, kinds
		delete(v, "mysqlServices")
		v["mode"], v["installationId"] = "PREVIEW", "selection-test"
		v["preparation"], v["setup"] = map[string]any{"initialize": false}, map[string]any{"action": "NONE"}
	})
	o, err := New(data, cat, location())
	if err != nil {
		t.Fatal(err)
	}
	return o
}
