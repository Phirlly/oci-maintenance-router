package ocistorage

import (
	"testing"
	"time"

	"oci-maintenance-router/internal/configuration"
	"oci-maintenance-router/internal/delivery"
	"oci-maintenance-router/internal/noticeledger"
	"oci-maintenance-router/internal/notices"
	"oci-maintenance-router/internal/routing"
)

const stateNoticeID = "ocid1.announcement.oc1..storageexample"

func stateClock() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) }
func noticeState(t *testing.T) noticeledger.Record {
	t.Helper()
	settings := []byte(`{"schemaVersion":1,"environmentName":"example","tenancyId":"ocid1.tenancy.oc1..example","deploymentRegion":"us-ashburn-1","regionMap":{"us-ashburn-1":"iad"},"compartmentIds":["ocid1.compartment.oc1..example"],"mysqlServices":["Oracle HeatWave MySQL"],"ownership":{"kind":"freeform","key":"owner"},"destinations":[{"id":"payments","secretId":"ocid1.vaultsecret.oc1.iad.payments"}],"routes":[{"ownerValue":"payments","destinationId":"payments"}],"pagerDutyRegion":"US","intakeTopicId":"ocid1.onstopic.oc1.iad.intake","operatorTopicId":"ocid1.onstopic.oc1.iad.operator"}`)
	catalog := configuration.Catalog{Regions: map[string]string{"us-ashburn-1": "iad"}, MySQLServices: []string{"Oracle HeatWave MySQL"}}
	location := configuration.LedgerLocation{Namespace: "example", Bucket: "ledger"}
	o, e := noticeledger.New(settings, catalog, location)
	if e != nil {
		t.Fatal(e)
	}
	id, platform, compartment := "ocid1.mysqldbsystem.oc1.iad.example", "IAAS", "ocid1.compartment.oc1..example"
	n, e := notices.Normalize(notices.Record{NoticeID: stateNoticeID, AnnouncementType: "SCHEDULED_MAINTENANCE", Services: catalog.MySQLServices, Platform: &platform, Resources: []notices.Resource{{ResourceID: id}}})
	if e != nil {
		t.Fatal(e)
	}
	lookup := map[string]routing.Lookup{id: {State: routing.Found, Metadata: routing.Metadata{ID: &id, CompartmentID: &compartment, FreeformTags: map[string]string{"owner": "payments"}}}}
	r, e := o.Observe(noticeledger.Record{}, n, false, lookup, stateClock())
	if e != nil {
		t.Fatal(e)
	}
	b, e := delivery.New(settings, catalog, location)
	if e != nil {
		t.Fatal(e)
	}
	drafts, e := b.Build(n, false, lookup, []string{id})
	if e != nil {
		t.Fatal(e)
	}
	r, e = r.Prepare(stateClock(), delivery.Preparation{Draft: drafts.Drafts[0], SecretVersion: 7})
	if e != nil {
		t.Fatal(e)
	}
	return r
}
func noticeStore(t *testing.T, c *Client, r noticeledger.Record) *NoticeStore {
	t.Helper()
	env, _ := r.Identity()
	s, e := NewNoticeStore(c, env)
	if e != nil {
		t.Fatal(e)
	}
	return s
}
func holdState(t *testing.T, r noticeledger.Record, reason delivery.Reason) noticeledger.Record {
	t.Helper()
	n, e := r.Hold(reason, stateClock())
	if e != nil {
		t.Fatal(e)
	}
	return n
}
