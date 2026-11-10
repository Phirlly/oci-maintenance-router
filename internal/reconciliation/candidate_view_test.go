package reconciliation

import (
	"reflect"
	"testing"

	"oci-maintenance-router/internal/notices"
)

func candidateView(t *testing.T, c *Candidates) CandidateView {
	t.Helper()
	v, e := c.ResolveView()
	if e != nil {
		t.Fatal(e)
	}
	return v
}

func TestCandidateViewRetainsCompleteRelevantHistoryAndSavedOnlyComponents(t *testing.T) {
	old, next := candidateNotice(t, 2, chain("first"), false), candidateNotice(t, 2, chain("second"), false)
	savedOnly := candidateNotice(t, 5, nil, false)
	ns := []notices.Notice{candidateNotice(t, 3, chain("second"), false), candidateNotice(t, 4, nil, false), candidateNotice(t, 1, chain("first"), false), candidateNotice(t, 6, nil, true)}
	c := candidates(t)
	observeCandidates(t, c, ns...)
	for _, r := range []struct{ ns []notices.Notice }{{[]notices.Notice{old, next}}, {[]notices.Notice{savedOnly}}} {
		if e := c.Restore(candidateRecord(t, r.ns...)); e != nil {
			t.Fatal(e)
		}
	}
	v := candidateView(t, c)
	groups := v.Components()
	if v.EnvironmentID() != c.environmentID || len(groups) != 3 {
		t.Fatal("lost environment or complete components")
	}
	if len(groups[0].Members) != 4 || len(groups[1].Members) != 1 || groups[1].Members[0].Revision != savedOnly.Key() || groups[2].Members[0].Revision != ns[3].Key() {
		t.Fatal("saved/bridged component membership lost")
	}
	want := map[notices.RevisionKey]string{old.Key(): "first", next.Key(): "second", ns[0].Key(): "second", ns[2].Key(): "first"}
	for _, m := range groups[0].Members {
		link, ok := want[m.Revision]
		if !ok || m.ChainID != link {
			t.Fatal("wrong historical member")
		}
		delete(want, m.Revision)
	}
	if len(want) != 0 {
		t.Fatal("missing history")
	}
	for _, g := range groups {
		if len(g.Fingerprint) != 64 {
			t.Fatal("missing fingerprint")
		}
		for i := 1; i < len(g.Members); i++ {
			a, b := g.Members[i-1], g.Members[i]
			if a.Revision.NoticeID > b.Revision.NoticeID || (a.Revision.NoticeID == b.Revision.NoticeID && a.Revision.Digest >= b.Revision.Digest) {
				t.Fatal("membership not canonical")
			}
		}
	}
	current := v.Current()
	if len(current) != 4 {
		t.Fatal("occurrences lost")
	}
	for i, m := range current {
		if m.Revision != ns[i].Key() || m.KnownCandidate != (i != 1) {
			t.Fatal("current order/relevance changed")
		}
	}
	legacy, e := c.Resolve()
	if e != nil || !reflect.DeepEqual(legacy, current) {
		t.Fatal("Resolve compatibility changed", e)
	}
}

func TestCandidateViewDistinguishesEmptyValidAndInvalid(t *testing.T) {
	empty := candidateView(t, candidates(t))
	if empty.EnvironmentID() == "" || empty.Current() == nil || empty.Components() == nil || len(empty.Current()) != 0 || len(empty.Components()) != 0 {
		t.Fatal("empty valid view missing identity")
	}
	for _, c := range []*Candidates{nil, {}} {
		v, e := c.ResolveView()
		candidateError(t, e, InvalidRequest)
		if v.EnvironmentID() != "" || v.Current() != nil || v.Components() != nil {
			t.Fatal("invalid view usable")
		}
	}
	var zero CandidateView
	if zero.EnvironmentID() != "" || zero.Current() != nil || zero.Components() != nil {
		t.Fatal("zero view has evidence")
	}
}
