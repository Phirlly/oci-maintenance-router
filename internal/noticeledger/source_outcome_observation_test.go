package noticeledger

import (
	"encoding/json"
	"testing"
	"time"
)

func TestObservationRecordsSourceOutcomeAtMergeTime(t *testing.T) {
	src := source(db)
	src.TimeUpdated = text("2030-01-01T00:00:00Z")
	o := observer(t, nil)
	n := normalized(t, src)
	capture, err := o.Capture(n)
	if err != nil {
		t.Fatal(err)
	}
	for _, apply := range []func(Record, time.Time) (Record, error){
		func(r Record, at time.Time) (Record, error) { return o.Observe(r, n, false, nil, at) },
		capture.Apply,
	} {
		var r Record
		for _, at := range []time.Time{clock(), clock().Add(time.Hour)} {
			r, err = apply(r, at)
			if err != nil {
				t.Fatal(err)
			}
			var wire struct {
				SchemaVersion int `json:"schemaVersion"`
				SourceOutcome struct {
					State          string `json:"state"`
					RecordedAt     string `json:"recordedAt"`
					RevisionDigest string `json:"revisionDigest"`
					HTTPStatus     int    `json:"httpStatus"`
				} `json:"sourceOutcome"`
			}
			if err := json.Unmarshal(encoded(t, r), &wire); err != nil {
				t.Fatal(err)
			}
			outcome := wire.SourceOutcome
			if wire.SchemaVersion != 2 || outcome.State != "AVAILABLE" || outcome.RecordedAt != at.Format(timeFormat) || outcome.RevisionDigest != n.Key().Digest || outcome.HTTPStatus != 0 {
				t.Fatalf("source evidence or merge clock missing: %+v", wire)
			}
			reload(t, r)
		}
	}
}
