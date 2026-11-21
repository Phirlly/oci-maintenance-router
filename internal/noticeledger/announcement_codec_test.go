package noticeledger

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"oci-maintenance-router/internal/delivery"
)

const instance = "ocid1.instance.oc1.iad.example"

func announcementWire(t *testing.T, name string) []byte {
	t.Helper()
	body, err := os.ReadFile("testdata/" + name + "-v4.json")
	if err != nil {
		t.Fatal(err)
	}
	return bytes.TrimSpace(body)
}

func announcementRecord(t *testing.T, name string) Record {
	t.Helper()
	body := announcementWire(t, name)
	var identity struct {
		EnvironmentID string `json:"environmentId"`
	}
	if err := json.Unmarshal(body, &identity); err != nil {
		t.Fatal(err)
	}
	r, err := Decode(body, identity.EnvironmentID, noticeID)
	if err != nil {
		t.Fatal("independent notice4 rejected", err)
	}
	return r
}

func TestAnnouncementNoticeIndependentWireAndLegacyRoundtrip(t *testing.T) {
	for _, name := range []string{"source-only", "announcement-mysql", "announcement-compute", "announcement-mixed"} {
		t.Run(name, func(t *testing.T) {
			r := announcementRecord(t, name)
			if r.Snapshot().SchemaVersion != 4 || !bytes.Equal(encoded(t, r), announcementWire(t, name)) {
				t.Fatal("format rewritten")
			}
			if len(r.Snapshot().Batches) != 0 && r.data.delivery.SchemaVersion() != 3 {
				t.Fatal("wrong nested format")
			}
		})
	}
	for _, name := range []string{"source-only.json", "source-only-v2.json", "source-only-v3.json", "rejected-recovery-v3.json"} {
		body, err := os.ReadFile("testdata/" + name)
		if err != nil {
			t.Fatal(err)
		}
		body = bytes.TrimSpace(body)
		r, err := Decode(body, "c081fc9b5129be4e85c6039722858f2454d79fcd9219504f473d53dffd712378", noticeID)
		if err != nil || !bytes.Equal(body, encoded(t, r)) {
			t.Fatal("legacy changed", name, err)
		}
	}
}

func TestAnnouncementNoticeKeepsClosedVersionAndResourceAdmission(t *testing.T) {
	r := announcementRecord(t, "source-only")
	env, id := r.Identity()
	for _, resource := range []string{db, instance} {
		body := changedWire(t, r, func(w *wireRecord) { w.Revisions[0].Resources[0].ResourceID = resource })
		if _, err := Decode(body, env, id); err != nil {
			t.Fatal("valid unassigned resource rejected", err)
		}
	}
	for _, resource := range []string{"ocid1.volume.oc1.iad.example", "ocid1.instance.oc2.iad.example", "ocid1.instance.oc1..example"} {
		body := changedWire(t, r, func(w *wireRecord) { w.Revisions[0].Resources[0].ResourceID = resource })
		bad, err := Decode(body, env, id)
		rejected(t, bad, err)
	}
	for _, version := range []int{0, 1, 2, 3, 5} {
		body := changedWire(t, r, func(w *wireRecord) { w.SchemaVersion = version; w.Revisions[0].Resources[0].ResourceID = instance })
		bad, err := Decode(body, env, id)
		rejected(t, bad, err)
	}
	for _, name := range []string{"announcement-mysql", "announcement-mixed"} {
		r := announcementRecord(t, name)
		env, id := r.Identity()
		// Every current lower enclosing version rejects delivery3, independently
		// of whether the saved resources are MySQL-only or mixed.
		for _, version := range []int{1, 2, 3, 5} {
			body := changedWire(t, r, func(w *wireRecord) { w.SchemaVersion = version })
			bad, err := Decode(body, env, id)
			rejected(t, bad, err)
		}
	}
	legacy := recoveryRecord(t, prepare(t, observed(t, source(db), db), preparations(t, source(db), db)...))
	body := changedWire(t, legacy, func(w *wireRecord) { w.SchemaVersion = 4 })
	env, id = legacy.Identity()
	bad, err := Decode(body, env, id)
	rejected(t, bad, err)
}

func TestAnnouncementNestedFixtureIsIndependentlyValid(t *testing.T) {
	// Diagnose fixture errors separately from the new enclosing-format admission.
	for _, name := range []string{"announcement-mysql", "announcement-compute", "announcement-mixed"} {
		var w struct {
			Delivery json.RawMessage `json:"delivery"`
		}
		if err := json.Unmarshal(announcementWire(t, name), &w); err != nil {
			t.Fatal(err)
		}
		d, err := delivery.DecodeRecord(w.Delivery, strings.Repeat("c", 64), noticeID)
		if err != nil || d.SchemaVersion() != 3 {
			t.Fatal("invalid independent delivery fixture", err)
		}
	}
}
