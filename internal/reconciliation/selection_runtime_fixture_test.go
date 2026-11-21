package reconciliation

import (
	"encoding/json"
	"testing"

	"oci-maintenance-router/internal/noticeledger"
	"oci-maintenance-router/internal/notices"
)

const resourceCatalogWire = `{"schemaVersion":1,"regions":{"us-ashburn-1":"iad","us-phoenix-1":"phx"},"resourceServices":{"mysqldbsystem":["Oracle HeatWave MySQL","MySQL"],"instance":["Oracle Cloud Infrastructure Compute"]}}`
const resourceInstance = "ocid1.instance.oc1.iad.example"

func resourceConfig(t *testing.T, base []byte, kinds ...string) []byte {
	t.Helper()
	var v map[string]any
	if err := json.Unmarshal(base, &v); err != nil {
		t.Fatal(err)
	}
	v["schemaVersion"], v["resourceTypes"] = 9, kinds
	delete(v, "mysqlServices")
	if v["setup"] == nil {
		v["setup"] = map[string]any{"action": "NONE"}
	}
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func selectedDiscovery(t *testing.T, ns []notices.Notice, records []noticeledger.Record) (*Discovery, *discoveryObjects, *Lease) {
	t.Helper()
	legacy, objects, lease := discoveryFixture(t, ns, records)
	cfg := resourceConfig(t, preparationConfig(t), "mysqldbsystem", "instance")
	objects.seed("configuration/router.json", cfg)
	objects.seed("configuration/catalog.json", []byte(resourceCatalogWire))
	d, err := NewDiscovery(cfg, []byte(resourceCatalogWire), candidateLocation(), legacy.collector.announcements, objects)
	if err != nil {
		t.Fatal(err)
	}
	d.now, d.wait = legacy.now, legacy.wait
	return d, objects, lease
}

func announcementRecordFor(t *testing.T, old noticeledger.Record) noticeledger.Record {
	t.Helper()
	var err error
	if old.Snapshot().SchemaVersion == 2 {
		old, err = old.MigrateRecovery(leaseTime())
	}
	if err == nil && old.Snapshot().SchemaVersion == 3 {
		old, err = old.MigrateAnnouncements(leaseTime())
	}
	if err != nil {
		t.Fatal(err)
	}
	return old
}
