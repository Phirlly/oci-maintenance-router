package reconciliation

import (
	"reflect"
	"testing"

	"oci-maintenance-router/internal/notices"
)

func TestCandidateViewSnapshotsAndConcurrentReadsAreIndependent(t *testing.T) {
	c := candidates(t)
	n := candidateNotice(t, 1, chain("linked"), true)
	observeCandidates(t, c, n)
	v := candidateView(t, c)
	want := v.Components()
	matches := v.Current()
	if len(want) != 1 || len(matches) != 1 {
		t.Fatal("missing captured membership")
	}
	results := make(chan bool, 8)
	for i := 0; i < 8; i++ {
		go func() {
			groups := v.Components()
			current := v.Current()
			same := reflect.DeepEqual(groups, want) && reflect.DeepEqual(current, matches)
			groups[0].Fingerprint = "changed"
			groups[0].Members[0].ChainID = "changed"
			groups[0].Members[0].Revision.NoticeID = "changed"
			current[0].KnownCandidate = false
			results <- same
		}()
	}
	for i := 0; i < 8; i++ {
		if !<-results {
			t.Fatal("snapshot mutation reached shared view")
		}
	}
	observeCandidates(t, c, candidateNotice(t, 2, chain("linked"), false))
	later := candidateView(t, c)
	if len(later.Components()[0].Members) != 2 || !reflect.DeepEqual(want, v.Components()) || !reflect.DeepEqual(matches, v.Current()) {
		t.Fatal("later admission changed prior view")
	}
	candidateError(t, c.Observe(notices.Notice{}), InvalidNotice)
	wantNoCandidateView(t, c, InvalidNotice)
	if !reflect.DeepEqual(want, v.Components()) {
		t.Fatal("failed accumulator invalidated prior snapshot")
	}
}

func wantNoCandidateView(t *testing.T, c *Candidates, code Code) {
	t.Helper()
	v, e := c.ResolveView()
	candidateError(t, e, code)
	if v.EnvironmentID() != "" || v.Current() != nil || v.Components() != nil {
		t.Fatal("partial view after invalid admission")
	}
}
