package delivery

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"
)

// Produce structurally consistent saved messages to test cross-batch invariants,
// which cannot be fabricated through the public Draft constructor.
func changedMembership(d *recordData, b savedBatch, id, destination string) savedBatch {
	previousID, previousDedup := b.Message.Resources[0].ResourceID, b.Message.DedupKey
	b.Message.Resources = []savedResource{{ResourceID: id, Region: "us-ashburn-1"}}
	b.Message.DestinationID = destination
	s := d.snapshot(b).Message
	b.Message.BatchID = batchID(d.EnvironmentID, s.Revision, destination, s.Resources)
	b.Message.DedupKey = "omr1:" + b.Message.BatchID
	b.Message.Payload = strings.ReplaceAll(strings.ReplaceAll(b.Message.Payload, previousID, id), previousDedup, b.Message.DedupKey)
	b.Message.PayloadDigest = digest([]byte(b.Message.Payload))
	return b
}

func sortedBatches(d *recordData) {
	slices.SortFunc(d.Batches, func(a, b savedBatch) int { return strings.Compare(a.Message.BatchID, b.Message.BatchID) })
}

func TestReloadRejectsOverlappingAndExcessiveAssignments(t *testing.T) {
	r, _ := codecFixture(t, "sample")
	original := r.data.Batches[0]
	d := r.copy()
	d.Batches = append(d.Batches, changedMembership(&d, original, original.Message.Resources[0].ResourceID, "other"))
	sortedBatches(&d)
	if _, err := decodeData(d); err == nil || err.Error() != "assignments: overlap" {
		t.Fatalf("overlap accepted: %v", err)
	}
	d = r.copy()
	d.Batches = nil
	for i := 0; i < 257; i++ {
		d.Batches = append(d.Batches, changedMembership(&d, original, fmt.Sprintf("ocid1.mysqldbsystem.oc1.iad.example%03d", i), "payments"))
	}
	sortedBatches(&d)
	if _, err := decodeData(d); err == nil || err.Error() != "assignments: too_large" {
		t.Fatalf("assignment limit not enforced: %v", err)
	}
	d.Batches = d.Batches[:256]
	if _, err := decodeData(d); err != nil {
		t.Fatalf("256 assignments rejected: %v", err)
	}
}

func TestReloadRejectsDuplicateUnsortedAndEmptyBatches(t *testing.T) {
	r, _ := codecFixture(t, "sample")
	for _, change := range []func(*recordData){
		func(d *recordData) { d.Batches = nil },
		func(d *recordData) { d.Batches = append(d.Batches, d.Batches[0]) },
		func(d *recordData) { d.Batches[0].Message.Resources = nil },
		func(d *recordData) { d.Batches[0].Message.Resources = make([]savedResource, 51) },
		func(d *recordData) {
			d.Batches[0].State = ReviewRequired
			d.Batches[0].NextAttemptAt = timeText(savedTime(nil))
		},
	} {
		d := r.copy()
		change(&d)
		rejectedData(t, d)
	}
	_, other := codecFixture(t, "other")
	both, err := r.Prepare(other)
	if err != nil {
		t.Fatal(err)
	}
	d := both.copy()
	slices.Reverse(d.Batches)
	rejectedData(t, d)
	// Same fields in a different order are intentionally not a supported codec.
	data, _ := json.Marshal(map[string]any{"batches": r.data.Batches, "noticeId": r.data.NoticeID, "environmentId": r.data.EnvironmentID, "schemaVersion": 1})
	if _, err := DecodeRecord(data, r.data.EnvironmentID, r.data.NoticeID); err == nil {
		t.Fatal("alternate record order accepted")
	}
}
