package reconciliation

import (
	"reflect"
	"testing"

	"oci-maintenance-router/internal/noticeledger"
	"oci-maintenance-router/internal/notices"
)

func TestCollectionPreservesCurrentAndSavedOnlyComponentEvidence(t *testing.T) {
	n := candidateNotice(t, 1, nil, true)
	saved := candidateNotice(t, 2, chain("old"), false)
	a, s := detailSources(t, []notices.Notice{n, n}, []noticeledger.Record{candidateRecord(t, saved)})
	got, e := collector(t, a, s).collect(budget(t), noPause)
	if e != nil || got.Candidates.EnvironmentID() == "" || len(got.Candidates.Components()) != 2 || !reflect.DeepEqual(got.Current, got.Candidates.Current()) {
		t.Fatal("collector discarded complete membership", e)
	}
	if len(got.Observations) != 2 || len(got.Saved) != 1 || got.Candidates.Components()[1].Members[0].Revision != saved.Key() {
		t.Fatal("saved-only evidence or original observations lost")
	}
	got.Current[0].KnownCandidate = false
	if !got.Candidates.Current()[0].KnownCandidate {
		t.Fatal("caller edited retained view")
	}
}
