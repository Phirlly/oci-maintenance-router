package reconciliation

import (
	"reflect"
	"strings"
	"testing"

	"oci-maintenance-router/internal/notices"
)

func TestCandidateFingerprintIgnoresOrderDuplicatesOriginAndPublication(t *testing.T) {
	a, b := candidateNotice(t, 1, chain("same"), true), candidateNotice(t, 2, chain("same"), false)
	c := candidates(t)
	observeCandidates(t, c, a, b, a)
	want := candidateView(t, c).Components()
	if len(want) != 1 {
		t.Fatal("missing relevant component")
	}
	changed := a.Snapshot()
	changed.TimeUpdated = chain("2026-02-01T00:00:00Z")
	published := normalizeCandidate(t, changed)
	for _, ns := range [][]notices.Notice{{b, a, b}, {a, published, b}} {
		c = candidates(t)
		observeCandidates(t, c, ns...)
		if !reflect.DeepEqual(want, candidateView(t, c).Components()) {
			t.Fatal("order/duplicate/publication changed fingerprint")
		}
	}
	data := strings.Replace(candidateConfig, `"key":"owner"`, `"key":"team"`, 1)
	c, e := NewCandidates([]byte(data), candidateCatalog(), candidateLocation())
	if e != nil {
		t.Fatal(e)
	}
	for _, n := range []notices.Notice{b, published} {
		if e = c.Restore(candidateRecord(t, n)); e != nil {
			t.Fatal(e)
		}
	}
	if !reflect.DeepEqual(want, candidateView(t, c).Components()) {
		t.Fatal("origin/routing generation changed membership")
	}
	observeCandidates(t, c, a, b)
	if !reflect.DeepEqual(want, candidateView(t, c).Components()) {
		t.Fatal("current/saved overlap changed fingerprint")
	}
}

func TestCandidateFingerprintCanonicalVector(t *testing.T) {
	// Independently pinned UTF8 compact JSON array: domain, schema1, 64 a's,
	// [[noticeID,64 b's,"\u003c\u003e\u0026\n🐬"]], no terminal newline.
	members := []CandidateRevision{{Revision: notices.RevisionKey{NoticeID: "ocid1.announcement.oc1..example1", Digest: strings.Repeat("b", 64)}, ChainID: "<>&\n🐬"}}
	const want = "4d49ff5dcedbe0a438ad652e80f2f731b54c102eff90a7a7d887fd203b3c688c"
	if got := componentFingerprint(strings.Repeat("a", 64), members); got != want {
		t.Fatalf("canonical fingerprint %s; want %s", got, want)
	}
}

func TestCandidateFingerprintChangesForEnvironmentRevisionAndMembership(t *testing.T) {
	n := candidateNotice(t, 1, chain("same"), true)
	c := candidates(t)
	observeCandidates(t, c, n)
	groups := candidateView(t, c).Components()
	if len(groups) != 1 {
		t.Fatal("missing component")
	}
	want := groups[0].Fingerprint
	changed := n.Snapshot()
	changed.Reference = chain("changed")
	for _, ns := range [][]notices.Notice{{n, normalizeCandidate(t, changed)}, {n, candidateNotice(t, 2, chain("same"), false)}, {candidateNotice(t, 1, chain("different"), true)}} {
		c = candidates(t)
		observeCandidates(t, c, ns...)
		if candidateView(t, c).Components()[0].Fingerprint == want {
			t.Fatal("changed membership kept fingerprint")
		}
	}
	other := candidateLocation()
	other.Bucket = "other"
	c, e := NewCandidates([]byte(candidateConfig), candidateCatalog(), other)
	if e != nil {
		t.Fatal(e)
	}
	observeCandidates(t, c, n)
	if candidateView(t, c).Components()[0].Fingerprint == want {
		t.Fatal("environment not bound")
	}
}
