package reconciliation

import (
	"testing"
	"time"

	"oci-maintenance-router/internal/configuration"
	"oci-maintenance-router/internal/noticeledger"
	"oci-maintenance-router/internal/notices"
)

const candidateConfig = `{"schemaVersion":1,"environmentName":"example","tenancyId":"ocid1.tenancy.oc1..example","deploymentRegion":"us-ashburn-1","regionMap":{"us-ashburn-1":"iad"},"compartmentIds":["ocid1.compartment.oc1..example"],"mysqlServices":["Oracle HeatWave MySQL"],"ownership":{"kind":"freeform","key":"owner"},"destinations":[{"id":"payments","secretId":"ocid1.vaultsecret.oc1.iad.payments"}],"routes":[{"ownerValue":"payments","destinationId":"payments"}],"pagerDutyRegion":"US","intakeTopicId":"ocid1.onstopic.oc1.iad.intake","operatorTopicId":"ocid1.onstopic.oc1.iad.operator"}`

func candidateCatalog() configuration.Catalog {
	return configuration.Catalog{Regions: map[string]string{"us-ashburn-1": "iad"}, MySQLServices: []string{"Oracle HeatWave MySQL"}}
}
func candidateLocation() configuration.LedgerLocation {
	return configuration.LedgerLocation{Namespace: "example", Bucket: "ledger"}
}
func candidates(t *testing.T) *Candidates {
	t.Helper()
	c, err := NewCandidates([]byte(candidateConfig), candidateCatalog(), candidateLocation())
	if err != nil {
		t.Fatal(err)
	}
	return c
}
func chain(s string) *string { return &s }
func candidateNotice(t *testing.T, id int, link *string, seed bool) notices.Notice {
	t.Helper()
	r := notices.Record{NoticeID: noticeID(id), AnnouncementType: "INFORMATION", ChainID: link}
	if seed {
		r.Services = []string{"Oracle HeatWave MySQL"}
	}
	return normalizeCandidate(t, r)
}
func normalizeCandidate(t *testing.T, r notices.Record) notices.Notice {
	t.Helper()
	n, err := notices.Normalize(r)
	if err != nil {
		t.Fatal(err)
	}
	return n
}
func observeCandidates(t *testing.T, c *Candidates, ns ...notices.Notice) {
	t.Helper()
	for _, n := range ns {
		if err := c.Observe(n); err != nil {
			t.Fatal(err)
		}
	}
}
func candidateRecord(t *testing.T, ns ...notices.Notice) noticeledger.Record {
	t.Helper()
	o, err := noticeledger.New([]byte(candidateConfig), candidateCatalog(), candidateLocation())
	if err != nil {
		t.Fatal(err)
	}
	var r noticeledger.Record
	for i, n := range ns {
		r, err = o.Observe(r, n, true, nil, time.Date(2026, 1, 1, 0, 0, i, 0, time.UTC))
		if err != nil {
			t.Fatal(err)
		}
	}
	return r
}
func requireMatches(t *testing.T, c *Candidates, ns []notices.Notice, known ...bool) []CandidateMatch {
	t.Helper()
	got, err := c.Resolve()
	if err != nil || len(got) != len(ns) || len(got) != len(known) {
		t.Fatalf("matches=%v err=%v; want %d", got, err, len(ns))
	}
	for i := range got {
		if got[i].Revision != ns[i].Key() || got[i].KnownCandidate != known[i] {
			t.Fatalf("match %d: %+v; want key=%+v known=%v", i, got[i], ns[i].Key(), known[i])
		}
	}
	return got
}
