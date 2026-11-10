package reconciliation

import (
	"testing"
	"time"

	"oci-maintenance-router/internal/delivery"
	"oci-maintenance-router/internal/noticeledger"
	"oci-maintenance-router/internal/notices"
)

func TestCollectionRetainsOriginalLaterRelevantObservations(t *testing.T) {
	early := candidateNotice(t, 1, chain("shared"), false)
	src := early.Snapshot()
	src.Reference = chain("original")
	early = normalizeCandidate(t, src)
	later := candidateNotice(t, 2, chain("shared"), true)
	unrelated := candidateNotice(t, 3, nil, false)
	a, s := detailSources(t, []notices.Notice{early, unrelated, later}, nil)
	got, e := collector(t, a, s).collect(budget(t), noPause)
	if e != nil || len(got.Observations) != 2 || len(got.Current) != 3 {
		t.Fatalf("missing captured occurrences: %d %v", len(got.Observations), e)
	}
	if got.Observations[0].Key() != early.Key() || got.Observations[1].Key() != later.Key() {
		t.Fatal("capture relevance/order changed")
	}
	r, e := got.Observations[0].Apply(noticeledger.Record{}, time.Now())
	if e != nil || *r.Snapshot().Revisions[0].Source.Reference != "original" {
		t.Fatal("original observation not available", e)
	}
	// detailSources rejects any additional Get: capture must not refetch.
}

func TestCollectionKeepsEveryOriginalRevisionAndPublicationOccurrence(t *testing.T) {
	first := candidateNotice(t, 1, nil, true)
	changed := first.Snapshot()
	changed.Reference = chain("changed")
	second := normalizeCandidate(t, changed)
	changed.TimeUpdated = chain("2026-02-01T00:00:00Z")
	third := normalizeCandidate(t, changed)
	a, s := detailSources(t, []notices.Notice{first, second, third}, nil)
	got, e := collector(t, a, s).collect(budget(t), noPause)
	if e != nil || len(got.Observations) != 3 {
		t.Fatal("source occurrence discarded", e)
	}
	var r noticeledger.Record
	at := time.Now()
	for i, o := range got.Observations {
		if o.Key() != got.Current[i].Revision {
			t.Fatal("capture/key misaligned")
		}
		r, e = o.Apply(r, at.Add(time.Duration(i)*time.Second))
		if e != nil {
			t.Fatal(e)
		}
	}
	view := r.Snapshot()
	if len(view.Revisions) != 2 || view.HoldReason != delivery.ReasonSourceChanged {
		t.Fatal("changed revision not retained/held")
	}
	for _, v := range view.Revisions {
		if v.Digest == second.Key().Digest && (v.Source.TimeUpdated == nil || *v.Source.TimeUpdated != "2026-02-01T00:00:00.000Z") {
			t.Fatal("publication-only observation lost")
		}
	}
}
