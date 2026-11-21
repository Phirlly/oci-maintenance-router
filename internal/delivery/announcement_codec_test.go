package delivery

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestAnnouncementRecordRoundtripAndLegacyAdmission(t *testing.T) {
	for _, tc := range []struct {
		category string
		ids      []string
	}{
		{"ACTION_REQUIRED", []string{announcementDB}},
		{"ACTION_REQUIRED", []string{announcementInstance, announcementDB}},
		{"SCHEDULED_MAINTENANCE", []string{announcementInstance, announcementDB}},
	} {
		r := announcementRecord(t, tc.category, "sample", tc.ids...)
		if r.SchemaVersion() != 3 {
			t.Fatal("new projection needs explicit schema3")
		}
		body, err := r.Encode()
		if err != nil {
			t.Fatal(err)
		}
		got, err := DecodeRecord(body, r.data.EnvironmentID, r.data.NoticeID)
		if err != nil {
			t.Fatal(err)
		}
		again, _ := got.Encode()
		if !bytes.Equal(body, again) {
			t.Fatal("reload rewrote payload/state")
		}
		for _, version := range []int{1, 2} {
			d := r.copy()
			d.SchemaVersion = version
			if version == 1 {
				d.Batches[0].AttemptKind = nil
			}
			rejectedData(t, d)
		}
	}
	legacy, p := codecFixture(t, "sample")
	for _, version := range []int{1, 2} {
		if version == 2 {
			var err error
			legacy, err = legacy.MigrateRecovery()
			if err != nil {
				t.Fatal(err)
			}
		}
		before, _ := legacy.Encode()
		newer := announcementPreparation(t, "ACTION_REQUIRED", "sample", announcementInstance)
		if got, err := legacy.Prepare(newer); err == nil || got.data != nil || err.Error() != "preparation: migration_required" {
			t.Fatal("implicit migration", err)
		}
		// Same batch identity retains the original even if a newer producer marker
		// requests a different format; only unseen work needs migration.
		newer.Draft.snapshot = p.Draft.snapshot
		newer.SecretVersion = 99
		got, err := legacy.Prepare(newer)
		if err != nil {
			t.Fatal(err)
		}
		after, _ := got.Encode()
		if !bytes.Equal(before, after) {
			t.Fatal("duplicate changed original")
		}
	}
}

func TestAnnouncementCodecRejectsCorruptionAfterRehash(t *testing.T) {
	r := announcementRecord(t, "ACTION_REQUIRED", "sample", announcementInstance)
	for _, replacement := range [][2]string{
		{`"formatVersion":2`, `"formatVersion":3`},
		{`"formatVersion":2`, `"formatVersion":null`},
		{`"formatVersion":2,`, ``},
		{`"formatVersion":2`, `"formatVersion":2,"formatVersion":2`},
		{`"announcementType":"ACTION_REQUIRED"`, `"announcementType":""`},
		{`"announcementType":"ACTION_REQUIRED"`, `"announcementType":null`},
		{`"announcementType":"ACTION_REQUIRED",`, ``},
		{`"event_action":"trigger"`, `"event_action":"resolve"`},
		{`"payload":{`, `"routing_key":"synthetic-value","payload":{`},
		{`"slot":1`, `"slot":3`},
		{`"reference":"sample"`, `"reference":"sample","extra":true`},
		{announcementInstance, announcementInstance + "other"},
	} {
		d := r.copy()
		m := &d.Batches[0].Message
		changed := strings.Replace(m.Payload, replacement[0], replacement[1], 1)
		if changed == m.Payload {
			t.Fatal("invalid corruption fixture", replacement)
		}
		m.Payload = changed
		m.PayloadDigest = digest([]byte(changed))
		rejectedData(t, d)
	}
	// Recompute batch identity and payload around invalid saved types/realms too.
	for _, id := range []string{"ocid1.volume.oc1.iad.example", "ocid1.instance.oc2.iad.example", "ocid1.instance.oc1..example"} {
		p := announcementPreparation(t, "ACTION_REQUIRED", "sample", id)
		d := r.copy()
		d.Batches[0].Message = saveMessage(p.Draft.snapshot)
		rejectedData(t, d)
	}
}

func TestAnnouncementSchemaPreservesLegacyBytesAndRejectsRelabeling(t *testing.T) {
	old, _ := codecFixture(t, "sample")
	old, err := old.MigrateRecovery()
	if err != nil {
		t.Fatal(err)
	}
	next, err := old.MigrateAnnouncements()
	if err != nil {
		t.Fatal(err)
	}
	before, _ := old.Encode()
	after, _ := next.Encode()
	want := bytes.Replace(before, []byte(`"schemaVersion":2`), []byte(`"schemaVersion":3`), 1)
	if !bytes.Equal(after, want) || !reflect.DeepEqual(old.Batches(), next.Batches()) {
		t.Fatal("migration rewrote saved work")
	}
	if _, err := decodeData(next.copy()); err != nil {
		t.Fatal(err)
	}
	// A legacy envelope cannot carry Compute resources even inside schema3.
	p := announcementPreparation(t, "SCHEDULED_MAINTENANCE", "sample", announcementInstance)
	d := next.copy()
	d.Batches[0].Message = saveMessage(p.Draft.snapshot)
	var wire map[string]any
	if err := json.Unmarshal([]byte(d.Batches[0].Message.Payload), &wire); err != nil {
		t.Fatal(err)
	}
	data := wire["payload"].(map[string]any)
	detail := data["custom_details"].(map[string]any)
	delete(detail, "formatVersion")
	delete(detail, "announcementType")
	data["summary"] = "OCI MySQL scheduled maintenance: 1 resource(s)"
	body, _ := json.Marshal(wire)
	d.Batches[0].Message.Payload = string(body)
	d.Batches[0].Message.PayloadDigest = digest(body)
	rejectedData(t, d)
}
