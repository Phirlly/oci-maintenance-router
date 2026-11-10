package noticeledger

import (
	"bytes"
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"oci-maintenance-router/internal/delivery"
)

func reload(t *testing.T, r Record) Record {
	t.Helper()
	env, id := r.Identity()
	b := encoded(t, r)
	n, e := Decode(b, env, id)
	if e != nil {
		t.Fatal(e)
	}
	if !reflect.DeepEqual(n.Snapshot(), r.Snapshot()) || !bytes.Equal(b, encoded(t, n)) || n.Follows(r) {
		t.Fatal("reload changed state or retained ephemeral parent")
	}
	return n
}
func TestWholeNoticeRoundTripAndIndependentGolden(t *testing.T) {
	r := observed(t, source(db))
	gold, e := os.ReadFile("testdata/source-only-v2.json")
	if e != nil {
		t.Fatal(e)
	}
	if !bytes.Equal(encoded(t, r), bytes.TrimSpace(gold)) {
		t.Fatal("source-only v2 format changed")
	}
	reload(t, r)
	r = prepare(t, observed(t, source(db), db), preparations(t, source(db), db)...)
	reload(t, r)
	r, ref := begin(t, r, r.Snapshot().Batches[0].Message.BatchID, clock())
	reload(t, r)
	for _, status := range []int{202, 400, 503, 0} {
		retry := time.Time{}
		if status == 0 || status == 503 {
			retry = clock()
		}
		n, e := r.Finish(ref, status, clock(), retry)
		if e != nil {
			t.Fatal(e)
		}
		reload(t, n)
	}
	n, e := r.Recover(clock().Add(time.Minute))
	if e != nil {
		t.Fatal(e)
	}
	reload(t, n)
	n, e = r.Hold(delivery.ReasonOperator, clock())
	if e != nil {
		t.Fatal(e)
	}
	reload(t, n)
	if _, e := (Record{}).Encode(); e == nil {
		t.Fatal("zero record encoded")
	}
}

func TestDecodeRejectsHostileOrNoncanonicalWholeRecords(t *testing.T) {
	r := observed(t, source(db))
	env, id := r.Identity()
	valid := encoded(t, r)
	for _, data := range [][]byte{nil, []byte("null"), []byte("{"), append([]byte(" "), valid...), append(append([]byte{}, valid...), []byte("{}")...), []byte{255}, []byte(strings.Repeat(" ", 1<<20+1)), []byte(strings.Repeat("[", 33) + strings.Repeat("]", 33)), []byte(`{"revisions":[` + strings.Repeat("null,", 8192) + `null]}`), []byte(`{"resources":[` + strings.Repeat("null,", 256) + `null]}`), bytes.Replace(valid, []byte(`"schemaVersion":2`), []byte(`"schemaVersion":2,"schemaVersion":2`), 1), bytes.Replace(valid, []byte(`"schemaVersion":2`), []byte(`"unknown":1,"schemaVersion":2`), 1), bytes.Replace(valid, []byte(`"chainId":null`), []byte(`"chainId":"\ud800"`), 1)} {
		n, e := Decode(data, env, id)
		rejected(t, n, e)
	}
	for _, ids := range [][2]string{{"", id}, {env, "ocid1.announcement.oc1..other"}} {
		n, e := Decode(valid, ids[0], ids[1])
		rejected(t, n, e)
	}
}

func changedWire(t *testing.T, r Record, mutate func(*wireRecord)) []byte {
	t.Helper()
	w := wireRecord{recordData: &recordData{}}
	if e := json.Unmarshal(encoded(t, r), &w); e != nil {
		t.Fatal(e)
	}
	mutate(&w)
	data, e := json.Marshal(w)
	if e != nil {
		t.Fatal(e)
	}
	return data
}
func TestDecodeRejectsCanonicalSemanticCorruption(t *testing.T) {
	r := prepare(t, observed(t, source(db), db), preparations(t, source(db), db)...)
	env, id := r.Identity()
	cases := map[string]func(*wireRecord){
		"version":             func(w *wireRecord) { w.SchemaVersion = 3 },
		"time":                func(w *wireRecord) { w.UpdatedAt = "0000-01-01T00:00:00.000Z" },
		"future_observation":  func(w *wireRecord) { w.Revisions[0].LastSeenAt = "2027-01-01T00:00:00.000Z" },
		"digest":              func(w *wireRecord) { w.Revisions[0].Digest = "bad" },
		"duplicate_revision":  func(w *wireRecord) { w.Revisions = append(w.Revisions, w.Revisions[0]) },
		"missing_revision":    func(w *wireRecord) { w.Revisions = nil },
		"source_time":         func(w *wireRecord) { w.Revisions[0].Source.TimeUpdated = text("not-a-date") },
		"unknown_hold":        func(w *wireRecord) { w.HoldReason = "private" },
		"unsynchronized_hold": func(w *wireRecord) { w.HoldReason = delivery.ReasonSourceChanged },
		"index":               func(w *wireRecord) { w.Revisions[0].Resources[0].Index = 9 },
		"unsafe_id":           func(w *wireRecord) { w.Revisions[0].Resources[0].ResourceID = "private" },
		"orphan_batch":        func(w *wireRecord) { w.Revisions[0].Resources[0].BatchID = strings.Repeat("f", 64) },
		"omitted_assignment": func(w *wireRecord) {
			w.Revisions[0].Resources[0].BatchID = ""
			w.Revisions[0].Resources[0].Disposition = "READY"
		},
		"region":           func(w *wireRecord) { w.Revisions[0].Resources[0].Region = "us-other-1" },
		"destination":      func(w *wireRecord) { w.Revisions[0].Resources[0].DestinationID = "video" },
		"false_receipt":    func(w *wireRecord) { w.Revisions[0].Resources[0].Disposition = "API_ACCEPTED" },
		"delivery_removed": func(w *wireRecord) { w.Delivery = nil },
		"delivery_corrupt": func(w *wireRecord) { w.Delivery = []byte(`{}`) },
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) { n, e := Decode(changedWire(t, r, change), env, id); rejected(t, n, e) })
	}
}
