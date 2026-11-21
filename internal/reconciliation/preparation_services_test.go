package reconciliation

import (
	"bytes"
	"context"
	"testing"

	"oci-maintenance-router/internal/notices"
	"oci-maintenance-router/internal/ociannouncements"
)

func TestPreparationServiceExpansionRetainsUnrecognizedResourceHistory(t *testing.T) {
	// The resource ID makes this notice a candidate before its label is accepted.
	old := normalizeCandidate(t, notices.Record{NoticeID: noticeID(1), AnnouncementType: "SCHEDULED_MAINTENANCE",
		Services: []string{"MySQL"}, Platform: chain("IAAS"), Resources: []notices.Resource{{ResourceID: "ocid1.mysqldbsystem.oc1.iad.example"}}})
	p, s, lease := preparationFixture(t, []notices.Notice{old})
	if err := p.Initialize(leaseBudget(t), lease); err != nil {
		t.Fatal(err)
	}
	first := prepareSuccessfully(t, p, lease)
	if !first.Suppresses("mysqldbsystem", "us-ashburn-1", baselineCompartment, old.Key()) {
		t.Fatal("unrecognized service with MySQL resource was omitted from initial history")
	}
	body, err := first.Encode()
	if err != nil {
		t.Fatal(err)
	}
	cfg := preparationConfig(t, func(c map[string]any) { c["mysqlServices"] = []string{"Oracle HeatWave MySQL", "MySQL"} })
	p = preparationFor(t, s, cfg, nil)
	// A covered scope must return before any new announcement scan.
	p.discovery.collector.announcements = announcementDetails{
		page: func(context.Context, string) (ociannouncements.Page, error) {
			t.Fatal("service expansion listed announcements in covered scope")
			return ociannouncements.Page{}, nil
		},
		get: func(context.Context, string) (notices.Notice, error) {
			t.Fatal("service expansion fetched an announcement in covered scope")
			return notices.Notice{}, nil
		},
	}
	before := preparationCalls(s, "LIST", "")
	retained := prepareSuccessfully(t, p, lease)
	actual, err := retained.Encode()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(body, actual) || preparationCalls(s, "LIST", "") != before {
		t.Fatal("service expansion rebuilt or rescanned covered history")
	}
}
