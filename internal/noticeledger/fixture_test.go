package noticeledger

import (
	"encoding/json"
	"testing"
	"time"

	"oci-maintenance-router/internal/configuration"
	"oci-maintenance-router/internal/delivery"
	"oci-maintenance-router/internal/notices"
	"oci-maintenance-router/internal/routing"
)

const db = "ocid1.mysqldbsystem.oc1.iad.example"
const otherDB = "ocid1.mysqldbsystem.oc1.iad.other"
const noticeID = "ocid1.announcement.oc1..example"
const configJSON = `{"schemaVersion":1,"environmentName":"example","tenancyId":"ocid1.tenancy.oc1..example","deploymentRegion":"us-ashburn-1","regionMap":{"us-ashburn-1":"iad"},"compartmentIds":["ocid1.compartment.oc1..example"],"mysqlServices":["Oracle HeatWave MySQL"],"ownership":{"kind":"freeform","key":"owner"},"destinations":[{"id":"payments","secretId":"ocid1.vaultsecret.oc1.iad.payments"},{"id":"video","secretId":"ocid1.vaultsecret.oc1.iad.video"}],"routes":[{"ownerValue":"payments","destinationId":"payments"},{"ownerValue":"video","destinationId":"video"}],"pagerDutyRegion":"US","intakeTopicId":"ocid1.onstopic.oc1.iad.intake","operatorTopicId":"ocid1.onstopic.oc1.iad.operator"}`

func text(s string) *string { return &s }
func clock() time.Time      { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) }
func catalog() configuration.Catalog {
	return configuration.Catalog{Regions: map[string]string{"us-ashburn-1": "iad"}, MySQLServices: []string{"Oracle HeatWave MySQL"}}
}
func location() configuration.LedgerLocation {
	return configuration.LedgerLocation{Namespace: "example", Bucket: "ledger"}
}
func settings(change func(map[string]any)) []byte {
	var v map[string]any
	_ = json.Unmarshal([]byte(configJSON), &v)
	if change != nil {
		change(v)
	}
	b, _ := json.Marshal(v)
	return b
}
func observer(t *testing.T, change func(map[string]any)) *Observer {
	t.Helper()
	o, err := New(settings(change), catalog(), location())
	if err != nil {
		t.Fatal(err)
	}
	return o
}
func source(ids ...string) notices.Record {
	r := notices.Record{NoticeID: noticeID, AnnouncementType: "SCHEDULED_MAINTENANCE", Services: []string{"Oracle HeatWave MySQL"}, Platform: text("IAAS")}
	for _, id := range ids {
		r.Resources = append(r.Resources, notices.Resource{ResourceID: id})
	}
	return r
}
func normalized(t *testing.T, r notices.Record) notices.Notice {
	t.Helper()
	n, err := notices.Normalize(r)
	if err != nil {
		t.Fatal(err)
	}
	return n
}
func lookups(ids ...string) map[string]routing.Lookup {
	m := map[string]routing.Lookup{}
	for _, id := range ids {
		m[id] = routing.Lookup{State: routing.Found, Metadata: routing.Metadata{ID: text(id), CompartmentID: text("ocid1.compartment.oc1..example"), FreeformTags: map[string]string{"owner": "payments"}}}
	}
	return m
}
func observed(t *testing.T, r notices.Record, ids ...string) Record {
	t.Helper()
	got, err := observer(t, nil).Observe(Record{}, normalized(t, r), false, lookups(ids...), clock())
	if err != nil {
		t.Fatal(err)
	}
	return got
}
func preparations(t *testing.T, r notices.Record, ids ...string) []delivery.Preparation {
	t.Helper()
	b, err := delivery.New(settings(nil), catalog(), location())
	if err != nil {
		t.Fatal(err)
	}
	result, err := b.Build(normalized(t, r), false, lookups(ids...), ids)
	if err != nil || len(result.Issues) != 0 {
		t.Fatalf("drafts: %v %v", err, result.Issues)
	}
	p := []delivery.Preparation{}
	for _, d := range result.Drafts {
		p = append(p, delivery.Preparation{Draft: d, SecretVersion: 7})
	}
	return p
}
func encoded(t *testing.T, r Record) []byte {
	t.Helper()
	b, e := r.Encode()
	if e != nil {
		t.Fatal(e)
	}
	return b
}
func rejected(t *testing.T, r Record, e error) {
	t.Helper()
	if e == nil || r.data != nil {
		t.Fatalf("usable failure: %v", e)
	}
}
