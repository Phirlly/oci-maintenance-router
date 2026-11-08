package notices

import (
	"errors"
	"reflect"
	"testing"
)

func text(value string) *string { return &value }

func sample() Record {
	return Record{
		NoticeID: "ocid1.announcement.oc1..example", AnnouncementType: "SCHEDULED_MAINTENANCE",
		ChainID: text("example-chain"), Reference: text("EXAMPLE-001"),
		Services: []string{"Oracle HeatWave MySQL"}, Platform: text("IAAS"),
		Summary: text("Synthetic scheduled maintenance"), Description: text("Within the configured window."),
		AdditionalInformation: text("Illustrative only."),
		Times: [2]SemanticTime{
			{Type: text("START_TIME"), Title: text("Maintenance week starts"), Value: text("2026-01-16T00:00:00Z")},
			{Type: text("END_TIME"), Title: text("Maintenance week ends"), Value: text("2026-01-22T23:59:59Z")},
		},
		Resources: []Resource{{
			ResourceID: "ocid1.mysqldbsystem.oc1.iad.example", ResourceName: text("example-db"),
			RegionDisplay:        text("US East (Ashburn)"),
			AdditionalProperties: []Property{{text("label"), text("Region")}, {text("value"), text("US East (Ashburn)")}},
		}},
		TimeCreated: text("2026-01-01T10:00:00Z"), TimeUpdated: text("2026-01-01T10:00:00.123456Z"),
		LifecycleState: text("INACTIVE"),
	}
}

func normalized(t *testing.T, input Record) Notice {
	t.Helper()
	got, err := Normalize(input)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func wantError(t *testing.T, input Record, field, code string) {
	t.Helper()
	got, err := Normalize(input)
	var detail *Error
	if !errors.As(err, &detail) || detail.Field != field || detail.Code != code {
		t.Fatalf("error=%v, want %s: %s", err, field, code)
	}
	if !reflect.DeepEqual(got, Notice{}) {
		t.Fatal("rejected record returned a partial notice")
	}
}
