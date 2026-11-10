package noticeledger

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestSourceOutcomeCodecRejectsInvalidStateClaims(t *testing.T) {
	r := observed(t, source(db))
	env, id := r.Identity()
	changes := map[string]func(*wireRecord){
		"legacy with outcome": func(w *wireRecord) { w.SchemaVersion = 1 },
		"missing outcome":     func(w *wireRecord) { w.SourceOutcome = nil },
		"unknown state":       func(w *wireRecord) { w.SourceOutcome.State = "private" },
		"empty time":          func(w *wireRecord) { w.SourceOutcome.RecordedAt = "" },
		"invalid time":        func(w *wireRecord) { w.SourceOutcome.RecordedAt = "0000-01-01T00:00:00.000Z" },
		"future time":         func(w *wireRecord) { w.SourceOutcome.RecordedAt = clock().Add(time.Second).Format(timeFormat) },
		"before revision":     func(w *wireRecord) { w.SourceOutcome.RecordedAt = clock().Add(-time.Second).Format(timeFormat) },
		"unknown digest":      func(w *wireRecord) { w.SourceOutcome.RevisionDigest = strings.Repeat("f", 64) },
		"missing digest":      func(w *wireRecord) { w.SourceOutcome.RevisionDigest = "" },
		"available status":    func(w *wireRecord) { w.SourceOutcome.HTTPStatus = 404 },
		"unknown with digest": func(w *wireRecord) { w.SourceOutcome.State = SourceUnknown },
		"unknown with status": func(w *wireRecord) {
			w.SourceOutcome.State, w.SourceOutcome.RevisionDigest, w.SourceOutcome.HTTPStatus = SourceUnknown, "", 404
		},
		"unavailable without hold": func(w *wireRecord) {
			w.SourceOutcome.State, w.SourceOutcome.RevisionDigest, w.SourceOutcome.HTTPStatus = SourceUnavailable, "", 404
		},
	}
	for name, change := range changes {
		t.Run(name, func(t *testing.T) {
			n, err := Decode(changedWire(t, r, change), env, id)
			rejected(t, n, err)
		})
	}
	lost := unavailable(t, r, 404, clock())
	for _, status := range []int{0, 200, 400, 501, 599} {
		n, err := Decode(changedWire(t, lost, func(w *wireRecord) { w.SourceOutcome.HTTPStatus = status }), env, id)
		rejected(t, n, err)
	}
	n, err := Decode(changedWire(t, lost, func(w *wireRecord) { w.SourceOutcome.RevisionDigest = r.Snapshot().Revisions[0].Digest }), env, id)
	rejected(t, n, err)
}

func TestSourceOutcomeCodecRequiresExactVersionedRepresentation(t *testing.T) {
	r := observed(t, source(db))
	env, id := r.Identity()
	b := encoded(t, r)
	outcome, _ := json.Marshal(r.Snapshot().SourceOutcome)
	explicitNull := bytes.Replace(b, outcome, []byte("null"), 1)
	n, err := Decode(explicitNull, env, id)
	rejected(t, n, err)
	for _, change := range [][2]string{
		{`"sourceOutcome":{`, `"sourceOutcome":null,"ignored":{`},
		{`"state":"AVAILABLE",`, ""},
		{`"recordedAt":"2026-01-01T00:00:00.000Z",`, ""},
		{`"httpStatus":0`, `"unknown":0,"httpStatus":0`},
		{`"httpStatus":0`, `"httpStatus":0,"httpStatus":0`},
		{`"httpStatus":0`, `"HTTPStatus":0`},
		{`"httpStatus":0`, `"httpStatus":0.0`},
		{`"httpStatus":0`, `"httpStatus":null`},
		{`,"httpStatus":0`, ""},
	} {
		input := bytes.Replace(b, []byte(change[0]), []byte(change[1]), 1)
		if bytes.Equal(input, b) {
			t.Fatal("corruption fixture did not change bytes")
		}
		n, err := Decode(input, env, id)
		rejected(t, n, err)
	}
	legacy := encoded(t, legacyRecord(t, r))
	legacy = bytes.Replace(legacy, []byte(`,"delivery":`), []byte(`,"sourceOutcome":null,"delivery":`), 1)
	n, err = Decode(legacy, env, id)
	rejected(t, n, err)
}
