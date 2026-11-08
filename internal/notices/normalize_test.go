package notices

import (
	"reflect"
	"testing"
)

func TestNormalizeMaintenancePreservesMeaning(t *testing.T) {
	input := sample()
	want := sample()
	want.Times[0].Value = text("2026-01-16T00:00:00.000Z")
	want.Times[1].Value = text("2026-01-22T23:59:59.000Z")
	want.TimeCreated = text("2026-01-01T10:00:00.000Z")
	want.TimeUpdated = text("2026-01-01T10:00:00.123Z")
	got := normalized(t, input)
	if !reflect.DeepEqual(got.Snapshot(), want) {
		t.Fatalf("normalized snapshot differs: %+v", got.Snapshot())
	}
	if got.Key().NoticeID != input.NoticeID || len(got.Key().Digest) != 64 {
		t.Fatal("missing revision identity")
	}
	if !reflect.DeepEqual(input, sample()) {
		t.Fatal("normalization mutated source")
	}
}

func TestMissingOptionalValuesAndUnknownLabels(t *testing.T) {
	input := Record{NoticeID: sample().NoticeID, AnnouncementType: "FUTURE_TYPE"}
	got := normalized(t, input).Snapshot()
	if got.Services == nil || got.Resources == nil || len(got.Resources) != 0 ||
		got.ChainID != nil || got.Summary != nil || got.TimeUpdated != nil ||
		got.Times != ([2]SemanticTime{}) || got.AnnouncementType != "FUTURE_TYPE" {
		t.Fatalf("optional values changed: %+v", got)
	}
	input.Platform = text("FUTURE_PLATFORM")
	input.Summary = text("")
	input.Times[0].Type = text("FUTURE_TIME")
	input.Resources = []Resource{{ResourceID: "unresolved-resource"}}
	got = normalized(t, input).Snapshot()
	if *got.Platform != "FUTURE_PLATFORM" || *got.Summary != "" ||
		*got.Times[0].Type != "FUTURE_TIME" || got.Resources[0].ResourceID != "unresolved-resource" ||
		got.Resources[0].AdditionalProperties == nil {
		t.Fatal("normalization inferred eligibility or lost unresolved context")
	}
}

func TestRequiredFieldsAndAnnouncementShape(t *testing.T) {
	for _, id := range []string{"", "sensitive-value", "ocid1.announcement.oc2..example",
		"ocid1.announcement.oc1.iad.example", "ocid1.announcement.oc1..",
		"ocid1.announcement.oc1....example", "ocid1.instance.oc1..example", "ocid1.announcement.oc1../example"} {
		input := sample()
		input.NoticeID = id
		wantError(t, input, "noticeId", "invalid")
	}
	for _, id := range []string{"ocid1.announcement.oc1...example", "ocid1.announcement.oc1..future.example-A_1"} {
		input := sample()
		input.NoticeID = id
		normalized(t, input)
	}
	input := sample()
	input.AnnouncementType = ""
	wantError(t, input, "announcementType", "required")
	input = sample()
	input.Resources[0].ResourceID = ""
	wantError(t, input, "resources.resourceId", "required")
	for _, field := range []string{"name", "value"} {
		input = sample()
		if field == "name" {
			input.Resources[0].AdditionalProperties[0].Name = nil
		} else {
			input.Resources[0].AdditionalProperties[0].Value = nil
		}
		wantError(t, input, "resources.additionalProperties."+field, "required")
	}
	input = sample()
	input.Resources[0].AdditionalProperties = []Property{{text(""), text("")}}
	normalized(t, input)
}

func TestZeroNoticeHasNoRevision(t *testing.T) {
	if (Notice{}).Key() != (RevisionKey{}) {
		t.Fatal("zero notice has an identity")
	}
}
