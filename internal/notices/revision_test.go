package notices

import (
	"bytes"
	"os"
	"slices"
	"testing"
)

func minimal() Record {
	return Record{NoticeID: "ocid1.announcement.oc1..example",
		AnnouncementType: "SCHEDULED_MAINTENANCE", Description: text("")}
}

func TestCanonicalGoldenBytesAndDigest(t *testing.T) {
	want, err := os.ReadFile("testdata/minimal-canonical.json")
	if err != nil {
		t.Fatal(err)
	}
	got := normalized(t, minimal())
	if !bytes.Equal(projection(got.Snapshot(), false), bytes.TrimSuffix(want, []byte("\n"))) {
		t.Fatal("canonical bytes differ from independently recorded fixture")
	}
	if got.Key() != (RevisionKey{minimal().NoticeID, "d094c646906a61133cbcecb3b5cb1078cb7fbd5d7c5b024fabd2be949f5ab0c0"}) {
		t.Fatalf("revision=%+v", got.Key())
	}
	input := minimal()
	input.Summary = text("<>&\u2028\u2029🐬")
	if !bytes.Contains(projection(normalized(t, input).Snapshot(), false),
		[]byte("\"summary\":\"\\u003c\\u003e\\u0026\\u2028\\u2029🐬\"")) {
		t.Fatal("v1 escaping contract changed")
	}
}

func TestMaintenanceGoldenBytesAndDigest(t *testing.T) {
	want, err := os.ReadFile("testdata/maintenance-canonical.json")
	if err != nil {
		t.Fatal(err)
	}
	got := normalized(t, sample())
	if !bytes.Equal(projection(got.Snapshot(), false), bytes.TrimSuffix(want, []byte("\n"))) ||
		got.Key().Digest != "da893badd39b21ff91f610032d735cbd596efeeaddcef222c316302a43eeeebf" {
		t.Fatal("maintenance canonical bytes or digest changed")
	}
}

func TestRevisionOrderIndependenceAndTimeEquivalence(t *testing.T) {
	input := sample()
	other := sample().Resources[0]
	other.ResourceID = "another-resource"
	input.Resources = append(input.Resources, other)
	input.Services = []string{"B", "A", "B"}
	before := normalized(t, input).Key()
	slices.Reverse(input.Resources)
	for i := range input.Resources {
		slices.Reverse(input.Resources[i].AdditionalProperties)
	}
	input.Services = []string{"A", "B"}
	input.Times[0].Value = text("2026-01-15T18:00:00-06:00")
	if normalized(t, input).Key() != before {
		t.Fatal("equivalent source content changed revision")
	}
}

func TestEveryContentFieldChangesRevision(t *testing.T) {
	for name, change := range map[string]func(*Record){
		"notice ID":              func(r *Record) { r.NoticeID = "ocid1.announcement.oc1..other" },
		"chain":                  func(r *Record) { r.ChainID = text("other") },
		"reference":              func(r *Record) { r.Reference = text("other") },
		"services":               func(r *Record) { r.Services = append(r.Services, "other") },
		"platform":               func(r *Record) { r.Platform = text("other") },
		"type":                   func(r *Record) { r.AnnouncementType = "other" },
		"summary":                func(r *Record) { r.Summary = text("other") },
		"description":            func(r *Record) { r.Description = text("other") },
		"additional information": func(r *Record) { r.AdditionalInformation = text("other") },
		"time one type":          func(r *Record) { r.Times[0].Type = text("other") },
		"time one label":         func(r *Record) { r.Times[0].Title = text("other") },
		"time one value":         func(r *Record) { r.Times[0].Value = text("2026-01-16T00:00:00.001Z") },
		"time two type":          func(r *Record) { r.Times[1].Type = text("other") },
		"time two label":         func(r *Record) { r.Times[1].Title = text("other") },
		"time two value":         func(r *Record) { r.Times[1].Value = nil },
		"resource ID":            func(r *Record) { r.Resources[0].ResourceID = "other" },
		"resource name":          func(r *Record) { r.Resources[0].ResourceName = text("other") },
		"resource region":        func(r *Record) { r.Resources[0].RegionDisplay = text("other") },
		"property name":          func(r *Record) { r.Resources[0].AdditionalProperties[0].Name = text("other") },
		"property value":         func(r *Record) { r.Resources[0].AdditionalProperties[0].Value = text("other") },
		"property multiplicity": func(r *Record) {
			r.Resources[0].AdditionalProperties = append(r.Resources[0].AdditionalProperties, r.Resources[0].AdditionalProperties[0])
		},
	} {
		t.Run(name, func(t *testing.T) {
			input := sample()
			before := normalized(t, input).Key()
			change(&input) // Publication timestamps deliberately remain unchanged.
			if normalized(t, input).Key().Digest == before.Digest {
				t.Fatal("changed source content reused the revision")
			}
		})
	}
}

func TestSourceContextDoesNotChangeRevision(t *testing.T) {
	input := sample()
	before := normalized(t, input).Key()
	input.LifecycleState = text("FUTURE_STATE")
	input.TimeCreated = nil
	input.TimeUpdated = text("2020-01-01T00:00:00Z")
	got := normalized(t, input)
	if got.Key() != before || *got.Snapshot().LifecycleState != "FUTURE_STATE" ||
		got.Snapshot().TimeCreated != nil || *got.Snapshot().TimeUpdated != "2020-01-01T00:00:00.000Z" {
		t.Fatal("source context lost or changed revision identity")
	}
	input = minimal()
	nilSummary := normalized(t, input).Key()
	input.Summary = text("")
	if normalized(t, input).Key() == nilSummary {
		t.Fatal("null and empty source text collapsed")
	}
}
