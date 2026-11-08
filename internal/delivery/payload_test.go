package delivery_test

import (
	"bytes"
	"reflect"
	"strings"
	"testing"

	"oci-maintenance-router/internal/notices"
	"oci-maintenance-router/internal/routing"
)

func TestPayloadHasOnlyRecipientResourcesAndApprovedSharedContext(t *testing.T) {
	other := db + "other"
	r := record(other, db)
	r.Reference = text("shared-reference<>&\u2028\u2029🐬")
	r.Times[0] = notices.SemanticTime{Type: text("START_TIME"), Title: text("Start of maintenance week"), Value: text("2026-01-15T18:00:00-06:00")}
	r.Times[1].Title = text("")
	r.Summary, r.Description, r.AdditionalInformation, r.ChainID = text("private-summary"), text("private-description"), text("private-information"), text("private-chain")
	r.Resources[0].ResourceName = text("private-name")
	r.Resources[0].AdditionalProperties = []notices.Property{{Name: text("private-property"), Value: text("private-value")}}
	n := notice(t, r)
	m := map[string]routing.Lookup{db: metadata(db, "payments"), other: metadata(other, "video"), db + "extra": metadata(db+"extra", "video")}
	got := build(t, builder(t, nil), n, m, other, db)
	if len(got.Drafts) != 2 || len(got.Issues) != 0 {
		t.Fatalf("wrong partition: %+v", got)
	}
	for i, destination := range []string{"payments", "video"} {
		s := got.Drafts[i].Snapshot()
		e := envelope(t, s)
		p := e["payload"].(map[string]any)
		c := p["custom_details"].(map[string]any)
		if len(e) != 3 || len(p) != 4 || len(c) != 5 || e["event_action"] != "trigger" || p["severity"] != "info" || p["summary"] != "OCI MySQL scheduled maintenance: 1 resource(s)" {
			t.Fatalf("unexpected envelope fields: %v", e)
		}
		if s.DestinationID != destination || c["noticeId"] != n.Key().NoticeID || c["revisionDigest"] != n.Key().Digest || c["reference"] != *r.Reference {
			t.Fatal("identity/shared context lost")
		}
		times := c["times"].([]any)
		if len(times) != 2 || !reflect.DeepEqual(times[0], map[string]any{"slot": float64(1), "type": "START_TIME", "title": "Start of maintenance week", "value": "2026-01-16T00:00:00.000Z"}) || !reflect.DeepEqual(times[1], map[string]any{"slot": float64(2), "type": nil, "title": "", "value": nil}) {
			t.Fatal("semantic/null time context changed")
		}
		id := db
		if i == 1 {
			id = other
		}
		if !reflect.DeepEqual(c["resources"], []any{map[string]any{"resourceId": id, "region": "us-ashburn-1"}}) {
			t.Fatal("wrong recipient resources")
		}
		body := string(s.PayloadWithoutRoutingKey)
		if strings.Contains(body, "private-") || strings.Contains(body, "vaultsecret") || strings.Contains(body, "routing_key") || strings.Contains(body, "Operations") || strings.Contains(body, db+"extra") {
			t.Fatal("unapproved source/credential field leaked")
		}
		if !bytes.Contains(s.PayloadWithoutRoutingKey, []byte(`shared-reference\u003c\u003e\u0026\u2028\u2029🐬`)) {
			t.Fatal("Go-v1 escaping changed")
		}
	}
}

func TestOptionalTimestampUsesUpdatedThenCreated(t *testing.T) {
	for _, tc := range []struct {
		created, updated *string
		want             any
	}{
		{nil, nil, nil},
		{text("2026-01-01T01:00:00+01:00"), nil, "2026-01-01T00:00:00.000Z"},
		{text("2026-01-01T00:00:00Z"), text("2026-01-02T00:00:00.123456Z"), "2026-01-02T00:00:00.123Z"},
		{nil, text("2026-01-02T00:00:00Z"), "2026-01-02T00:00:00.000Z"},
	} {
		r := record(db)
		r.TimeCreated, r.TimeUpdated = tc.created, tc.updated
		s := single(t, build(t, builder(t, nil), notice(t, r), map[string]routing.Lookup{db: metadata(db, "payments")}, db))
		p := envelope(t, s)["payload"].(map[string]any)
		value, exists := p["timestamp"]
		if value != tc.want || exists != (tc.want != nil) {
			t.Fatalf("timestamp = %v, exists %v", value, exists)
		}
	}
}
