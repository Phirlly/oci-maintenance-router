package delivery

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"testing"
	"time"
)

// Independent worst-size metadata fixture. Valid states use subsets of these
// fields; this deliberately impossible combination measures the admission bound.
func maximumMetadata(d recordData) int {
	stamp := "9999-12-31T23:59:59.999Z"
	for i := range d.Batches {
		b := &d.Batches[i]
		b.State = ReviewRequired
		b.AttemptCount = math.MaxUint64
		b.OwnerToken = strings.Repeat("a", 128)
		b.FirstAttemptAt = &stamp
		b.LastAttemptAt = &stamp
		b.AttemptExpiresAt = &stamp
		b.UpdatedAt = &stamp
		b.NextAttemptAt = &stamp
		b.AcceptedAt = &stamp
		b.Reason = ReasonConfigurationChanged
		b.Ambiguous = false
	}
	data, _ := json.Marshal(d)
	return len(data)
}

func TestAdmissionReservesExactEncodedTransitionCapacity(t *testing.T) {
	var preparations []Preparation
	// Fill with distinct revisions of the same notice; each has independent
	// membership. The final revision lets us hit the byte boundary exactly.
	for i := 0; i < 32; i++ {
		_, p := codecFixture(t, fmt.Sprintf("%02d", i)+strings.Repeat("x", 30000))
		preparations = append(preparations, p)
	}
	base, err := NewRecord(preparations...)
	if err != nil {
		t.Fatal(err)
	}
	_, tail := codecFixture(t, "tail<>&🐬")
	candidate, err := base.Prepare(tail)
	if err != nil {
		t.Fatal(err)
	}
	room := (1 << 20) - maximumMetadata(candidate.copy())
	if room < 0 || room > 30000 {
		t.Fatalf("invalid boundary fixture: %d", room)
	}
	_, tail = codecFixture(t, "tail<>&🐬"+strings.Repeat("x", room))
	full, err := base.Prepare(tail)
	if err != nil {
		t.Fatal(err)
	}
	if maximumMetadata(full.copy()) != 1<<20 {
		t.Fatal("admission bound not exact")
	}
	// Exercise actual transitions near the bound, not only the size fixture.
	now := time.Date(9999, 12, 30, 23, 0, 0, 0, time.UTC)
	for _, saved := range full.Batches() {
		var ref AttemptRef
		full, ref, err = full.Begin(saved.Message.BatchID, AttemptContext{OwnerToken: strings.Repeat("a", 128), RoutingGeneration: saved.Message.RoutingGeneration, Now: now, LeaseExpiresAt: now.Add(time.Minute)})
		if err != nil {
			t.Fatal(err)
		}
		full, err = full.Finish(ref, 0, now, now)
		if err != nil {
			t.Fatal(err)
		}
	}
	full, err = full.Recover(now.Add(24 * time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if data, _ := full.Encode(); len(data) > 1<<20 {
		t.Fatal("outcomes exhausted reserved capacity")
	}
	_, over := codecFixture(t, "tail<>&🐬"+strings.Repeat("x", room+1))
	if got, err := base.Prepare(over); err == nil || got.data != nil {
		t.Fatal("one byte over admitted")
	}
	if _, err := decodeData(full.copy()); err != nil {
		t.Fatal(err)
	}
	if len(base.Batches()) != 32 {
		t.Fatal("capacity failure changed base")
	}
	// An artificially low-metadata record still must be rejected on reload if
	// it lacks transition headroom, even though its current bytes fit.
	d := full.copy()
	// Replace the tail with its one-byte-larger counterpart, keeping current
	// bytes below 1 MiB but violating required future-state headroom.
	for i := range d.Batches {
		if d.Batches[i].Message.BatchID == tail.Draft.snapshot.BatchID {
			d.Batches[i] = savedBatch{Message: saveMessage(over.Draft.snapshot), SecretVersion: 7, State: Prepared}
		}
	}
	sortedBatches(&d)
	if data, _ := json.Marshal(d); len(data) > 1<<20 {
		t.Fatal("fixture must fit before reserving metadata")
	}
	if _, err := decodeData(d); err == nil || err.Error() != "record: too_large" {
		t.Fatalf("reload skipped headroom: %v", err)
	}
}
