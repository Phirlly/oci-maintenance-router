package reconciliation

import (
	"strings"
	"testing"

	"oci-maintenance-router/internal/notices"
)

func TestSavedCandidateAdmissionIsAtomicAcrossRevisions(t *testing.T) {
	c := candidates(t)
	large := candidateNotice(t, 0, chain(strings.Repeat("x", 600000)), true)
	for range 6 {
		observeCandidates(t, c, large)
	}
	r := candidateRecord(t,
		candidateNotice(t, 1, chain(strings.Repeat("a", 300000)), true),
		candidateNotice(t, 1, chain(strings.Repeat("b", 300000)), true))
	beforeCount, beforeBytes := len(c.facts), c.bytes
	candidateError(t, c.Restore(r), ByteLimit)
	if len(c.facts) != beforeCount || c.bytes != beforeBytes {
		t.Fatal("first saved revision was retained when second exceeded capacity")
	}
}

func TestCandidateByteBoundaryCombinesSavedAndCurrentFacts(t *testing.T) {
	base := candidateNotice(t, 0, nil, true)
	size := 4*1024*1024/8 - len(base.Key().NoticeID) - len(base.Key().Digest)
	n := candidateNotice(t, 0, chain(strings.Repeat("x", size)), true)
	r := candidateRecord(t, n)
	c := candidates(t)
	for range 7 {
		observeCandidates(t, c, n)
	}
	if err := c.Restore(r); err != nil {
		t.Fatal(err)
	}
	requireMatches(t, c, []notices.Notice{n, n, n, n, n, n, n}, true, true, true, true, true, true, true)
	candidateError(t, c.Restore(r), ByteLimit)
}
