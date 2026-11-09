package ocimysql

import (
	"encoding/json"
	"os"
	"testing"

	"oci-maintenance-router/internal/configuration"
	"oci-maintenance-router/internal/notices"
	"oci-maintenance-router/internal/routing"
)

func testPolicy(t *testing.T, kind string, scope []string) *routing.Policy {
	t.Helper()
	data, err := os.ReadFile("testdata/config.json")
	if err != nil {
		t.Fatal(err)
	}
	var value map[string]any
	if err = json.Unmarshal(data, &value); err != nil {
		t.Fatal(err)
	}
	if kind == "freeform" {
		value["ownership"] = map[string]any{"kind": "freeform", "key": "Owner"}
	}
	value["compartmentIds"] = scope
	data, err = json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	p, err := routing.New(data, configuration.Catalog{Regions: map[string]string{"us-ashburn-1": "iad"}, MySQLServices: []string{"Oracle HeatWave MySQL"}})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func testNotice(t *testing.T, id string) notices.Notice {
	t.Helper()
	platform := "IAAS"
	n, err := notices.Normalize(notices.Record{NoticeID: "ocid1.announcement.oc1..example", AnnouncementType: "SCHEDULED_MAINTENANCE",
		Platform: &platform, Services: []string{"Oracle HeatWave MySQL"}, Resources: []notices.Resource{{ResourceID: id}}})
	if err != nil {
		t.Fatal(err)
	}
	return n
}
