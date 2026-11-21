package delivery_test

import (
	"bytes"
	"testing"

	"oci-maintenance-router/internal/delivery"
	"oci-maintenance-router/internal/routing"
)

func TestSelectedBuilderKeepsScheduledMySQLPayloadAndDedupBytes(t *testing.T) {
	cat := catalog()
	cat.SchemaVersion, cat.ResourceServices = 1, map[string][]string{"mysqldbsystem": cat.MySQLServices, "instance": {"Oracle Cloud Infrastructure Compute"}}
	cat.MySQLServices = nil
	data := settings(t, func(v map[string]any) {
		v["schemaVersion"], v["resourceTypes"] = 9, []string{"mysqldbsystem"}
		delete(v, "mysqlServices")
		v["mode"], v["installationId"] = "PREVIEW", "selected-test"
		v["preparation"], v["setup"] = map[string]any{"initialize": false}, map[string]any{"action": "NONE"}
	})
	selected, err := delivery.New(data, cat, ledger())
	if err != nil {
		t.Fatal(err)
	}
	n := notice(t, record(db))
	lookups := map[string]routing.Lookup{db: metadata(db, "payments")}
	before := single(t, build(t, builder(t, nil), n, lookups, db))
	after := single(t, build(t, selected, n, lookups, db))
	if before.BatchID != after.BatchID || before.DedupKey != after.DedupKey || before.PayloadDigest != after.PayloadDigest || !bytes.Equal(before.PayloadWithoutRoutingKey, after.PayloadWithoutRoutingKey) {
		t.Fatal("selected policy changed legacy payload or dedup identity")
	}
	if before.RoutingGeneration == after.RoutingGeneration {
		t.Fatal("fixture did not exercise distinct routing compatibility")
	}
}
