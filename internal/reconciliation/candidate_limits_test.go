package reconciliation

import (
	"strings"
	"testing"

	"oci-maintenance-router/internal/notices"
)

func TestCandidateOccurrenceLimitIncludesDuplicatesAndSavedRevisions(t *testing.T) {
	n := candidateNotice(t, 0, nil, true)
	r := candidateRecord(t, n, candidateNotice(t, 0, chain("changed"), true))
	for _, saved := range []bool{false, true} {
		c := candidates(t)
		count := 10000
		if saved {
			count -= 2
			if err := c.Restore(r); err != nil {
				t.Fatal(err)
			}
		}
		for range count {
			observeCandidates(t, c, n)
		}
		got, err := c.Resolve()
		if err != nil || len(got) != count || !got[count-1].KnownCandidate {
			t.Fatalf("exact occurrence cap failed: count=%d err=%v", len(got), err)
		}
		candidateError(t, c.Observe(n), ItemLimit)
		wantNoCandidateView(t, c, ItemLimit)
		got, err = c.Resolve()
		candidateError(t, err, ItemLimit)
		if got != nil {
			t.Fatal("over-cap result was partial")
		}
	}
}

func TestCandidateByteLimitCountsExactUTF8Bytes(t *testing.T) {
	base := candidateNotice(t, 0, nil, false)
	keyBytes := len(base.Key().NoticeID) + len(base.Key().Digest)
	perChain := 4*1024*1024/8 - keyBytes
	link := strings.Repeat("é", perChain/2) + strings.Repeat("x", perChain%2)
	for _, overflow := range []bool{false, true} {
		c := candidates(t)
		n := candidateNotice(t, 0, chain(link), false)
		for range 7 {
			observeCandidates(t, c, n)
		}
		last := n
		if overflow {
			last = candidateNotice(t, 0, chain(link+"x"), false)
			candidateError(t, c.Observe(last), ByteLimit)
			wantNoCandidateView(t, c, ByteLimit)
			got, err := c.Resolve()
			candidateError(t, err, ByteLimit)
			if got != nil {
				t.Fatal("byte overflow returned partial result")
			}
		} else {
			observeCandidates(t, c, last)
			requireMatches(t, c, []notices.Notice{n, n, n, n, n, n, n, n}, false, false, false, false, false, false, false, false)
		}
	}
}

func TestSavedCandidateAdmissionCannotExceedRemainingBudgets(t *testing.T) {
	n := candidateNotice(t, 0, nil, true)
	r := candidateRecord(t, n, candidateNotice(t, 0, chain("changed"), true))
	c := candidates(t)
	for range 9999 {
		observeCandidates(t, c, n)
	}
	candidateError(t, c.Restore(r), ItemLimit)
	wantNoCandidateView(t, c, ItemLimit)
	got, err := c.Resolve()
	candidateError(t, err, ItemLimit)
	if got != nil {
		t.Fatal("saved admission returned partial result")
	}
	c = candidates(t)
	large := candidateNotice(t, 0, chain(strings.Repeat("x", 600000)), true)
	saved := candidateRecord(t, large)
	for range 6 {
		observeCandidates(t, c, large)
	}
	candidateError(t, c.Restore(saved), ByteLimit)
	wantNoCandidateView(t, c, ByteLimit)
	got, err = c.Resolve()
	candidateError(t, err, ByteLimit)
	if got != nil {
		t.Fatal("saved byte overflow returned partial result")
	}
}
