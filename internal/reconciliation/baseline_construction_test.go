package reconciliation

import (
	"testing"

	"oci-maintenance-router/internal/noticeledger"
	"oci-maintenance-router/internal/notices"
)

func TestBaselineRequiresCompleteDiscoveryAndValidID(t *testing.T) {
	got, err := (DiscoveryResult{}).Baseline("initial")
	baselineFailure(t, got, err, "invalid_result")
	r := baselineResult(t, nil, nil, nil, nil)
	for _, id := range []string{"", "../private", "private value"} {
		got, err := r.Baseline(id)
		baselineFailure(t, got, err, "invalid_id")
	}
}

func TestBaselineEmptyPassHasExactNonrecursiveScope(t *testing.T) {
	r := baselineResult(t, nil, nil, []string{"ocid1.tenancy.oc1..example", baselineCompartment}, []string{"us-phoenix-1", "us-ashburn-1"})
	b := buildBaseline(t, "initial", r)
	if b.ID() != "initial" || b.EnvironmentID() != r.EnvironmentID() || b.CompletedAt() != r.CompletedAt() || !b.Covers(r) {
		t.Fatal("baseline identity/scope lost")
	}
	layers := b.Layers()
	if len(layers) != 1 || len(layers[0].Cells) != 4 || layers[0].Revisions == nil || len(layers[0].Revisions) != 0 {
		t.Fatal("empty scope layer malformed")
	}
	child := baselineResult(t, nil, nil, []string{baselineOtherCompartment}, nil)
	if b.Covers(child) {
		t.Fatal("root was treated as recursive")
	}
}

func TestBaselineIncludesOnlyCurrentCandidatePairs(t *testing.T) {
	a := candidateNotice(t, 1, nil, true)
	src := a.Snapshot()
	src.Summary = chain("current")
	changed := normalizeCandidate(t, src)
	unrelated := candidateNotice(t, 2, nil, false)
	unavailable := candidateNotice(t, 3, nil, true)
	r := baselineResult(t, []notices.Notice{changed, unrelated, changed}, []noticeledger.Record{candidateRecord(t, a), candidateRecord(t, unavailable)}, nil, nil)
	b := buildBaseline(t, "initial", r)
	layers := b.Layers()
	if len(layers) != 1 || len(layers[0].Revisions) != 1 || layers[0].Revisions[0].Digest != changed.Key().Digest {
		t.Fatal("baseline relabeled unrelated or saved history as current")
	}
	if !b.Suppresses("us-ashburn-1", baselineCompartment, changed.Key()) || b.Suppresses("us-phoenix-1", baselineCompartment, changed.Key()) || b.Suppresses("us-ashburn-1", baselineOtherCompartment, changed.Key()) || b.Suppresses("us-ashburn-1", baselineCompartment, a.Key()) {
		t.Fatal("suppression crossed exact cell/revision boundary")
	}
}
