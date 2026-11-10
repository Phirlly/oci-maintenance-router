package noticeledger

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"oci-maintenance-router/internal/delivery"
)

func maximumSize(t *testing.T, r Record) int {
	t.Helper()
	w := wireRecord{recordData: &recordData{}}
	if e := json.Unmarshal(encoded(t, r), &w); e != nil {
		t.Fatal(e)
	}
	w.HoldReason = delivery.ReasonConfigurationChanged
	if w.SchemaVersion == 2 {
		w.SourceOutcome = &SourceOutcome{State: SourceAvailable, RecordedAt: clock().Format(timeFormat), RevisionDigest: strings.Repeat("f", 64)}
	}
	for i := range w.Revisions {
		for j := range w.Revisions[i].Resources {
			d := &w.Revisions[i].Resources[j]
			d.Disposition = "REVIEW_REQUIRED"
			d.Reason = strings.Repeat("x", 32)
			d.BatchID = strings.Repeat("x", 64)
		}
	}
	actual := len(w.Delivery)
	data, _ := json.Marshal(w)
	if actual == 4 {
		return len(data)
	}
	reserved, e := r.data.delivery.ReservedBytes()
	if e != nil {
		t.Fatal(e)
	}
	return len(data) - actual + reserved
}
func TestCompleteObjectCapacityReservesOutcomesAndNeverEvicts(t *testing.T) {
	src := source(db)
	r := prepare(t, observed(t, src, db), preparations(t, src, db)...)
	r, ref := begin(t, r, r.Snapshot().Batches[0].Message.BatchID, clock())
	o := observer(t, nil)
	for i := 0; i < 31; i++ {
		src.Reference = text(fmt.Sprintf("%02d", i) + strings.Repeat("x", 30000))
		n, e := o.Observe(r, normalized(t, src), false, lookups(db), clock())
		if e != nil {
			t.Fatal(e)
		}
		r = n
	}
	src.Reference = text("tail<>&🐬")
	trial, e := o.Observe(r, normalized(t, src), false, lookups(db), clock())
	if e != nil {
		t.Fatal(e)
	}
	room := (1 << 20) - maximumSize(t, trial)
	if room < 0 || room > 100000 {
		t.Fatalf("bad capacity fixture %d", room)
	}
	src.Reference = text("tail<>&🐬" + strings.Repeat("x", room))
	full, e := o.Observe(r, normalized(t, src), false, lookups(db), clock())
	if e != nil {
		t.Fatal(e)
	}
	if maximumSize(t, full) != 1<<20 {
		t.Fatal("wrong complete reserved size")
	}
	accepted, e := full.Finish(ref, 202, clock(), time.Time{})
	if e != nil || len(encoded(t, accepted)) > 1<<20 {
		t.Fatal("reserved receipt cannot fit")
	}
	reload(t, accepted)
	recovered, e := full.Recover(clock().Add(time.Minute))
	if e != nil {
		t.Fatal(e)
	}
	reload(t, recovered)
	src.Reference = text(*src.Reference + "x")
	over, e := o.Observe(r, normalized(t, src), false, lookups(db), clock())
	rejected(t, over, e)
	if len(r.Snapshot().Revisions) != 32 || len(full.Snapshot().Batches) != 1 {
		t.Fatal("capacity check evicted history")
	}
}
