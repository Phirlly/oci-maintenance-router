package routing_test

import (
	"testing"

	"oci-maintenance-router/internal/routing"
)

const instance = "ocid1.instance.oc1.iad.example"
const computeService = "Oracle Cloud Infrastructure Compute"

func selectedPolicy(t *testing.T, kinds ...string) *routing.Policy {
	t.Helper()
	cat := catalog()
	cat.SchemaVersion, cat.ResourceServices = 1, map[string][]string{
		"mysqldbsystem": cat.MySQLServices, "instance": {computeService},
	}
	cat.MySQLServices = nil
	data := settings(t, func(v map[string]any) {
		v["schemaVersion"], v["resourceTypes"] = 9, kinds
		delete(v, "mysqlServices")
		v["mode"], v["installationId"] = "PREVIEW", "selection-test"
		v["preparation"] = map[string]any{"initialize": false}
		v["setup"] = map[string]any{"action": "NONE"}
	})
	p, err := routing.New(data, cat)
	if err != nil {
		t.Fatal("selected-resource policy rejected", err)
	}
	return p
}
