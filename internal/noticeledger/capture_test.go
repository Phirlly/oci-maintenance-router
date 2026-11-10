package noticeledger

import (
	"bytes"
	"strings"
	"testing"

	"oci-maintenance-router/internal/notices"
	"oci-maintenance-router/internal/routing"
)

func captured(t *testing.T, n notices.Notice) Observation {
	t.Helper()
	o, e := observer(t, nil).Capture(n)
	if e != nil {
		t.Fatal(e)
	}
	return o
}
func applyCaptured(t *testing.T, o Observation, r Record) Record {
	t.Helper()
	next, e := o.Apply(r, clock())
	if e != nil {
		t.Fatal(e)
	}
	return next
}
func TestCapturePreservesOnlyOriginalRedactedProjection(t *testing.T) {
	src := source(db)
	src.Reference = text("original-reference")
	src.ChainID = text("original-chain")
	src.Summary = text("private-summary")
	src.Description = text(strings.Repeat("private-body", 1000))
	src.AdditionalInformation = text("private-extra")
	src.Resources[0].ResourceName = text("private-name")
	src.Resources[0].AdditionalProperties = []notices.Property{{Name: text("private-tag"), Value: text("private-value")}}
	n := normalized(t, src)
	o := captured(t, n)
	if o.Key() != n.Key() || o.RetainedBytes() <= 0 || o.RetainedBytes() > 3000 {
		t.Fatal("original projection not retained or raw body counted")
	}
	*src.Reference = "changed"
	snapshot := n.Snapshot()
	*snapshot.ChainID = "changed"
	r := applyCaptured(t, o, Record{})
	view := r.Snapshot()
	if len(view.Revisions) != 1 || *view.Revisions[0].Source.Reference != "original-reference" || *view.Revisions[0].Source.ChainID != "original-chain" {
		t.Fatal("source context lost")
	}
	if view.Revisions[0].Resources[0].Disposition != string(routing.Unresolved) || len(view.Batches) != 0 {
		t.Fatal("capture invented ownership or delivery")
	}
	for _, private := range []string{"private-summary", "private-body", "private-extra", "private-name", "private-tag", "private-value"} {
		if bytes.Contains(encoded(t, r), []byte(private)) {
			t.Fatal("raw source retained", private)
		}
	}
	*view.Revisions[0].Source.Reference = "caller edit"
	view.Revisions[0].Resources[0].ResourceID = "caller edit"
	again := applyCaptured(t, o, Record{}).Snapshot()
	if *again.Revisions[0].Source.Reference != "original-reference" || again.Revisions[0].Resources[0].ResourceID != db {
		t.Fatal("captured projection aliased snapshot")
	}
}

func TestCaptureRetainsPotentialCandidateBeforeRelevanceIsKnown(t *testing.T) {
	src := source()
	src.Services = []string{"unrelated"}
	src.ChainID = text("later-related")
	o := captured(t, normalized(t, src))
	r := applyCaptured(t, o, Record{})
	if len(r.Snapshot().Revisions) != 1 || r.Snapshot().Revisions[0].Assessment != routing.ReviewRequired {
		t.Fatal("potential chain member discarded")
	}
}

func TestCaptureRejectsInvalidInputs(t *testing.T) {
	for _, o := range []*Observer{nil, {}} {
		got, e := o.Capture(normalized(t, source(db)))
		if e == nil || got.Key().NoticeID != "" || got.RetainedBytes() != 0 {
			t.Fatal("invalid observer captured")
		}
	}
	got, e := observer(t, nil).Capture(notices.Notice{})
	if e == nil || got.RetainedBytes() != 0 {
		t.Fatal("zero notice captured")
	}
	bad, e := (Observation{}).Apply(observed(t, source(db)), clock())
	rejected(t, bad, e)
}
