package reconciliation

import (
	"reflect"
	"testing"

	"oci-maintenance-router/internal/notices"
)

func TestBaselineExpansionPreservesOldScopeAndSpanningNoticeEligibility(t *testing.T) {
	old := candidateNotice(t, 1, nil, true)
	arrived := candidateNotice(t, 2, nil, true)
	first := baselineResult(t, []notices.Notice{old}, nil, nil, nil)
	base := buildBaseline(t, "original", first)
	current := baselineResult(t, []notices.Notice{old, arrived}, nil, []string{baselineOtherCompartment, baselineCompartment}, nil)
	expanded, err := base.Extend("expanded", current)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(base.Layers()[0], expanded.Layers()[0]) || len(base.Layers()) != 1 || len(expanded.Layers()) != 2 {
		t.Fatal("old layer changed during expansion")
	}
	if !expanded.Covers(current) || expanded.Suppresses("us-ashburn-1", baselineCompartment, arrived.Key()) || !expanded.Suppresses("us-ashburn-1", baselineOtherCompartment, arrived.Key()) {
		t.Fatal("new scope pairs suppressed existing scope")
	}
	removed := baselineResult(t, []notices.Notice{arrived}, nil, []string{baselineOtherCompartment}, nil)
	if !expanded.Covers(removed) {
		t.Fatal("removing scope lost remaining coverage")
	}
	got, err := expanded.Extend("removed", removed)
	baselineFailure(t, got, err, "no_new_scope")
	got, err = expanded.Extend("readded", current)
	baselineFailure(t, got, err, "no_new_scope")
	if expanded.Suppresses("us-ashburn-1", baselineCompartment, arrived.Key()) {
		t.Fatal("readding scope reset original history")
	}
}

func TestBaselineExpansionRejectsSameIDAndIncompleteResult(t *testing.T) {
	r := baselineResult(t, nil, nil, nil, nil)
	b := buildBaseline(t, "original", r)
	got, err := b.Extend("original", r)
	baselineFailure(t, got, err, "id_reused")
	got, err = b.Extend("next", DiscoveryResult{})
	baselineFailure(t, got, err, "invalid_result")
	got, err = (Baseline{}).Extend("next", r)
	baselineFailure(t, got, err, "invalid_record")
	if (Baseline{}).Covers(r) || b.Covers(DiscoveryResult{}) {
		t.Fatal("zero result/record supplied coverage")
	}
}
