package noticeledger

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"
)

func TestSelectedObserverCreatesCompleteAnnouncementRecords(t *testing.T) {
	o := selectedObserver(t, "mysqldbsystem", "instance")
	for _, ids := range [][]string{{db}, {selectedInstance}, {db, selectedInstance}} {
		r := source(ids...)
		r.Services = []string{"Oracle HeatWave MySQL", selectedComputeService}
		n := normalized(t, r)
		direct, err := o.Observe(Record{}, n, false, nil, clock())
		if err != nil {
			t.Fatal(err)
		}
		captured, err := o.Capture(n)
		if err != nil {
			t.Fatal(err)
		}
		got, err := captured.Apply(Record{}, clock())
		if err != nil || !bytes.Equal(encoded(t, direct), encoded(t, got)) {
			t.Fatal("capture differs from direct observation", err)
		}
		v := got.Snapshot()
		if v.SchemaVersion != 4 || v.Recovery == nil || v.Recovery.Actions == nil || len(v.Recovery.Actions) != 0 {
			t.Fatal("incomplete announcement record", v)
		}
		reload(t, got)
	}
}

func TestSelectedObservationRequiresExplicitExistingRecordMigration(t *testing.T) {
	o := selectedObserver(t, "mysqldbsystem")
	n := normalized(t, source(db))
	capture, err := o.Capture(n)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range []Record{observed(t, source(db)), recoveryRecord(t, observed(t, source(db)))} {
		before := encoded(t, r)
		bad, err := o.Observe(r, n, true, nil, clock())
		rejected(t, bad, err)
		bad, err = capture.Apply(r, clock())
		rejected(t, bad, err)
		bad, _, err = o.ApproveRecovery(r, RecoveryRequest{}, n, true, nil, clock())
		rejected(t, bad, err)
		if e, ok := err.(*Error); !ok || e.Code != "migration_required" {
			t.Fatal("approval skipped explicit migration", err)
		}
		if !bytes.Equal(before, encoded(t, r)) {
			t.Fatal("failed adoption mutated predecessor")
		}
	}
	old := recoveryRecord(t, observed(t, source(db)))
	up, err := old.MigrateAnnouncements(clock())
	if err != nil {
		t.Fatal(err)
	}
	got, err := capture.Apply(up, clock())
	if err != nil || !got.Follows(up) || got.Snapshot().SchemaVersion != 4 {
		t.Fatal("explicit migration not adopted", err)
	}
}

func TestSelectedObserverPreservesRedactedClosedIdentity(t *testing.T) {
	src := source(selectedInstance)
	src.Services = []string{selectedComputeService}
	base, err := observer(t, nil).Observe(Record{}, normalized(t, src), true, nil, clock())
	if err != nil {
		t.Fatal(err)
	}
	old := recoveryRecord(t, base)
	q := closeRequest(t, old, CloseRevision, old.Snapshot().Revisions[0].Digest)
	closed, action, err := old.CloseRecovery(q, clock())
	if err != nil || action.Status != RecoveryClosed {
		t.Fatal(err)
	}
	row := closed.Snapshot().Revisions[0].Resources[0]
	if row.ResourceID != "" || row.Disposition != "CLOSED" {
		t.Fatal("fixture not redacted closed row", row)
	}
	up, err := closed.MigrateAnnouncements(clock())
	if err != nil {
		t.Fatal(err)
	}
	got, err := selectedObserver(t, "instance").Observe(up, normalized(t, src), true, lookups(selectedInstance), clock())
	if err != nil || !reflect.DeepEqual(got.Snapshot().Revisions[0].Resources[0], row) {
		t.Fatal("closed identity reopened or refused", err)
	}
	reload(t, got)
}

func TestLegacyCaptureRetainsExactEncoding(t *testing.T) {
	o := captured(t, normalized(t, source(db)))
	want, err := json.Marshal(struct {
		EnvironmentID string   `json:"environmentId"`
		NoticeID      string   `json:"noticeId"`
		Revision      Revision `json:"revision"`
	}{o.data.EnvironmentID, o.data.NoticeID, o.data.Revision})
	if err != nil || len(want) != o.RetainedBytes() {
		t.Fatal("legacy capture bytes changed", err)
	}
}
