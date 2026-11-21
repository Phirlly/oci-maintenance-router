package reconciliation

import (
	"fmt"
	"strings"
	"testing"
)

func TestBaselineTypedScopeRejectsMissingUnknownOrAmbiguousTypes(t *testing.T) {
	base := buildBaseline(t, "legacy", baselineResult(t, nil, nil, nil, nil))
	for _, types := range [][]string{nil, {}, {""}, {"volume"}, {"instance", "instance"}, {"mysqldbsystem", "instance"}, {"instance", "mysqldbsystem", "volume"}} {
		r := resourceBaselineResult(t, types)
		b, err := r.Baseline("invalid")
		baselineFailure(t, b, err, "invalid_result")
		if base.Covers(r) {
			t.Fatal("invalid type scope claimed coverage")
		}
		if cells, err := r.scope.addedCells(nil); err == nil || cells != nil {
			t.Fatal("invalid type scope expanded")
		}
	}
	if cells, err := (discoveryScope{}).addedCells(nil); err == nil || cells != nil {
		t.Fatal("zero scope expanded")
	}
}

func TestBaselineTypedCellsHonorExactWireCapacity(t *testing.T) {
	r := resourceBaselineResult(t, []string{"instance"})
	small := buildBaseline(t, "boundary", r)
	r.scope.compartments = []string{baselineCompartment + strings.Repeat("a", baselineLimit-len(baselineWire(t, small)))}
	exact := buildBaseline(t, "boundary", r)
	wire := baselineWire(t, exact)
	if len(wire) != baselineLimit {
		t.Fatal("typed capacity fixture is not exact", len(wire))
	}
	restarted, err := DecodeBaseline(wire, exact.EnvironmentID(), exact.ID())
	if err != nil || !restarted.Covers(r) {
		t.Fatal("exact typed capacity rejected", err)
	}
	b, err := r.Baseline("boundaryx")
	baselineFailure(t, b, err, "too_large")
	r.scope.compartments[0] += strings.Repeat("a", 1024)
	b, err = r.Baseline("boundary")
	baselineFailure(t, b, err, "too_large")
}

func TestBaselineCartesianCapacityIncludesResourceTypes(t *testing.T) {
	r := resourceBaselineResult(t, []string{"instance", "mysqldbsystem"})
	base := buildBaseline(t, "both", r)
	r.scope.compartments = make([]string, 17000)
	for i := range r.scope.compartments {
		r.scope.compartments[i] = fmt.Sprintf("ocid1.compartment.oc1..c%05d", i)
	}
	b, err := r.Baseline("too-many")
	baselineFailure(t, b, err, "too_large")
	if base.Covers(r) {
		t.Fatal("typed Cartesian capacity was ignored")
	}
}
