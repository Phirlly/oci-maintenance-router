package reconciliation

import (
	"bytes"
	"testing"

	"oci-maintenance-router/internal/notices"
)

func TestCandidateConfigurationAndResultsAreOwned(t *testing.T) {
	data := []byte(candidateConfig)
	catalog := candidateCatalog()
	c, err := NewCandidates(data, catalog, candidateLocation())
	if err != nil {
		t.Fatal(err)
	}
	copy(data, bytes.Repeat([]byte("x"), len(data)))
	catalog.MySQLServices[0] = "changed"
	catalog.Regions["us-ashburn-1"] = "changed"
	first := candidateNotice(t, 0, chain("same"), false)
	seed := candidateNotice(t, 1, chain("same"), true)
	observeCandidates(t, c, first)
	prior := requireMatches(t, c, []notices.Notice{first}, false)
	observeCandidates(t, c, seed)
	requireMatches(t, c, []notices.Notice{first, seed}, true, true)
	if prior[0].KnownCandidate {
		t.Fatal("later resolve mutated earlier result")
	}
	prior[0].Revision.NoticeID = "caller changed"
	prior[0].KnownCandidate = true
	requireMatches(t, c, []notices.Notice{first, seed}, true, true)
}

func TestCandidateInstancesRemainIndependent(t *testing.T) {
	for i := range 12 {
		t.Run(noticeID(i), func(t *testing.T) {
			t.Parallel()
			c := candidates(t)
			n := candidateNotice(t, i, chain("shared-text"), i%2 == 0)
			observeCandidates(t, c, n)
			requireMatches(t, c, []notices.Notice{n}, i%2 == 0)
		})
	}
}
