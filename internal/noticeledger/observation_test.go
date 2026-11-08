package noticeledger

import (
	"bytes"
	"reflect"
	"testing"
	"time"

	"oci-maintenance-router/internal/notices"
	"oci-maintenance-router/internal/routing"
)

func TestSourceOnlyObservationPreservesEverySafeDecision(t *testing.T) {
	r := source("private malformed", db, "ocid1.mysqldbsystem.oc1.unknown.example")
	r.Description = text("private description")
	r.Summary = text("private summary")
	r.Resources[1].ResourceName = text("private name")
	r.Reference = text("reference")
	r.ChainID = text("opaque/chain")
	got := observed(t, r)
	s := got.Snapshot()
	if s.NoticeID != noticeID || len(s.Revisions) != 1 || len(s.Batches) != 0 {
		t.Fatal("notice without batches was lost")
	}
	v := s.Revisions[0]
	if v.Digest != normalized(t, r).Key().Digest || len(v.Resources) != 3 || *v.Source.Reference != "reference" || *v.Source.ChainID != "opaque/chain" {
		t.Fatal("source context or membership lost")
	}
	for i, d := range v.Resources {
		if d.Index != i || d.Disposition != "UNRESOLVED" {
			t.Fatal("incorrect indexed disposition")
		}
	}
	if v.Resources[2].ResourceID != "" || v.Resources[2].Reason != string(routing.ReasonResourceID) || v.Resources[1].Region != "" {
		t.Fatal("unsafe ID or guessed region")
	}
	for _, private := range []string{"private malformed", "private description", "private summary", "private name"} {
		if bytes.Contains(encoded(t, got), []byte(private)) {
			t.Fatal("private source content persisted")
		}
	}
	s.Revisions[0].Resources[0].Reason = "mutated"
	*s.Revisions[0].Source.Reference = "mutated"
	if got.Snapshot().Revisions[0].Resources[0].Reason == "mutated" || *got.Snapshot().Revisions[0].Source.Reference == "mutated" {
		t.Fatal("snapshot aliases state")
	}
}

func TestRepeatedObservationRefreshesContextWithoutInventingRevision(t *testing.T) {
	r := source(db)
	r.TimeUpdated = text("2026-01-01T00:00:00Z")
	old := observed(t, r)
	before := encoded(t, old)
	r.TimeUpdated = text("2026-01-02T00:00:00Z")
	r.LifecycleState = text("INACTIVE")
	next, err := observer(t, nil).Observe(old, normalized(t, r), false, lookups(db), clock().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	s := next.Snapshot()
	if len(s.Revisions) != 1 || s.Revisions[0].Resources[0].Disposition != "READY" || s.Revisions[0].FirstSeenAt == s.Revisions[0].LastSeenAt || *s.Revisions[0].Source.TimeUpdated != "2026-01-02T00:00:00.000Z" {
		t.Fatal("repeat observation lost context or routing")
	}
	if !bytes.Equal(before, encoded(t, old)) {
		t.Fatal("observation mutated prior state")
	}
}

func TestUnsupportedCandidatesRemainVisibleAndUnrelatedNoticesAreRejected(t *testing.T) {
	r := source(db)
	r.AnnouncementType = "SERVICE_DISRUPTION"
	got := observed(t, r, db).Snapshot()
	if len(got.Revisions) != 1 || got.Revisions[0].Assessment != routing.ReviewRequired || got.Revisions[0].Resources[0].Disposition != "REVIEW_REQUIRED" {
		t.Fatal("unsupported candidate lost or ready")
	}
	r = source()
	r.Services = []string{"unrelated"}
	n, e := observer(t, nil).Observe(Record{}, normalized(t, r), false, nil, clock())
	rejected(t, n, e)
	n, e = observer(t, nil).Observe(Record{}, normalized(t, r), true, nil, clock())
	if e != nil || len(n.Snapshot().Revisions) != 1 {
		t.Fatal("known candidate lost")
	}
}

func TestObservationRejectsInvalidIdentityClockAndConfiguration(t *testing.T) {
	base := observed(t, source(db))
	r := source(db)
	r.NoticeID = "ocid1.announcement.oc1..different"
	n, e := observer(t, nil).Observe(base, normalized(t, r), false, nil, clock())
	rejected(t, n, e)
	for _, at := range []time.Time{{}, clock().Add(-time.Second), time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC)} {
		n, e = observer(t, nil).Observe(base, normalized(t, source(db)), false, nil, at)
		rejected(t, n, e)
	}
	for _, o := range []*Observer{nil, {}} {
		n, e = o.Observe(base, normalized(t, source(db)), false, nil, clock())
		rejected(t, n, e)
	}
	n, e = observer(t, nil).Observe(base, notices.Notice{}, false, nil, clock())
	rejected(t, n, e)
	o := observer(t, func(v map[string]any) { v["tenancyId"] = "ocid1.tenancy.oc1..other" })
	n, e = o.Observe(base, normalized(t, source(db)), false, nil, clock())
	rejected(t, n, e)
	if _, e := New([]byte(`{}`), catalog(), location()); e == nil {
		t.Fatal("invalid config accepted")
	}
	if !reflect.DeepEqual(base.Snapshot(), observed(t, source(db)).Snapshot()) {
		t.Fatal("failed observation changed record")
	}
}
