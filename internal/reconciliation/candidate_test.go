package reconciliation

import (
	"testing"

	"oci-maintenance-router/internal/notices"
	"oci-maintenance-router/internal/routing"
)

func TestCandidatesPreserveOrderAndDuplicateObservations(t *testing.T) {
	c := candidates(t)
	unrelated := candidateNotice(t, 0, nil, false)
	seed := candidateNotice(t, 1, nil, true)
	ns := []notices.Notice{unrelated, seed, seed, unrelated}
	observeCandidates(t, c, ns...)
	requireMatches(t, c, ns, false, true, true, false)
}

func TestCandidatesUseRoutingSeedsWithoutAuthorizingDelivery(t *testing.T) {
	for _, tc := range []struct {
		name string
		r    notices.Record
		want bool
	}{
		{"service", notices.Record{Services: []string{"Oracle HeatWave MySQL"}}, true},
		{"resource-prefix", notices.Record{Resources: []notices.Resource{{ResourceID: "ocid1.mysqldbsystem.invalid"}}}, true},
		{"different-service", notices.Record{Services: []string{"mysql"}}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := candidates(t)
			tc.r.NoticeID = noticeID(0)
			tc.r.AnnouncementType = "INFORMATION"
			n := normalizeCandidate(t, tc.r)
			observeCandidates(t, c, n)
			got := requireMatches(t, c, []notices.Notice{n}, tc.want)
			assessment, targets := c.policy.Targets(n, got[0].KnownCandidate)
			if len(targets) != 0 || assessment.State == routing.SupportedShape {
				t.Fatal("candidate relevance bypassed supported-shape checks")
			}
		})
	}
}

func TestCandidatesEmptyAndSavedOnlyHaveNoCurrentMatches(t *testing.T) {
	c := candidates(t)
	requireMatches(t, c, nil)
	if err := c.Restore(candidateRecord(t, candidateNotice(t, 0, chain("prior"), true))); err != nil {
		t.Fatal(err)
	}
	requireMatches(t, c, nil)
}
