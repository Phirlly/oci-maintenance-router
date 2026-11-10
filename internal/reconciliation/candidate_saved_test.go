package reconciliation

import (
	"strings"
	"testing"

	"oci-maintenance-router/internal/notices"
)

func TestSavedCandidatesSeedEveryHistoricalRevisionChain(t *testing.T) {
	r := candidateRecord(t, candidateNotice(t, 1, chain("old"), false), candidateNotice(t, 1, chain("new"), false))
	for _, savedFirst := range []bool{false, true} {
		c := candidates(t)
		ns := []notices.Notice{candidateNotice(t, 2, chain("old"), false), candidateNotice(t, 3, chain("new"), false), candidateNotice(t, 1, nil, false), candidateNotice(t, 4, nil, false)}
		if savedFirst {
			if err := c.Restore(r); err != nil {
				t.Fatal(err)
			}
		}
		observeCandidates(t, c, ns...)
		if !savedFirst {
			if err := c.Restore(r); err != nil {
				t.Fatal(err)
			}
		}
		requireMatches(t, c, ns, true, true, true, false)
	}
}

func TestSavedCandidateRelevanceSurvivesRoutingConfigurationChange(t *testing.T) {
	r := candidateRecord(t, candidateNotice(t, 1, chain("saved"), false))
	data := strings.Replace(candidateConfig, `"key":"owner"`, `"key":"team"`, 1)
	c, err := NewCandidates([]byte(data), candidateCatalog(), candidateLocation())
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Restore(r); err != nil {
		t.Fatal(err)
	}
	n := candidateNotice(t, 2, chain("saved"), false)
	observeCandidates(t, c, n)
	requireMatches(t, c, []notices.Notice{n}, true)
}
