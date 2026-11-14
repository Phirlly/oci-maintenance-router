package reconciliation

import "testing"

func TestBaselineSelectionRejectsRewindAndRewrittenHistory(t *testing.T) {
	_, _, first, second := preparedStoreFixture(t)
	third, err := second.Extend("third", baselineResult(t, nil, nil, nil, []string{"us-phoenix-1"}))
	if err != nil {
		t.Fatal(err)
	}
	alter := func(b Baseline, change func(*baselineData)) Baseline {
		t.Helper()
		body := alteredBaseline(t, b, change)
		var id = b.ID()
		// Caller changes only content except for the explicit ID-reuse case below.
		got, err := DecodeBaseline(body, b.EnvironmentID(), id)
		if err != nil {
			t.Fatal("invalid fixture", err)
		}
		return got
	}
	changedPrevious := alter(first, func(d *baselineData) { d.Layers[0].Cells[0].CompartmentID = baselineOtherCompartment })
	changedCells := alter(third, func(d *baselineData) { d.Layers[0].Cells[0].CompartmentID = "ocid1.compartment.oc1..another" })
	changedTime := alter(third, func(d *baselineData) { d.Layers[0].CompletedAt = "2026-11-12T12:00:00.000Z" })
	changedPairs := alter(third, func(d *baselineData) {
		d.Layers[0].Revisions = []BaselineRevision{{"ocid1.announcement.oc1..other", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}}
	})
	reordered := alter(third, func(d *baselineData) { d.Layers[0], d.Layers[1] = d.Layers[1], d.Layers[0] })
	reusedBody := alteredBaseline(t, third, func(d *baselineData) { d.BaselineID = first.ID() })
	reused, err := DecodeBaseline(reusedBody, first.EnvironmentID(), first.ID())
	if err != nil {
		t.Fatal(err)
	}
	foreignBody := alteredBaseline(t, third, func(d *baselineData) {
		d.EnvironmentID = "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
	})
	foreign, err := DecodeBaseline(foreignBody, "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", third.ID())
	if err != nil {
		t.Fatal(err)
	}
	lateral := buildBaseline(t, "lateral", baselineResult(t, nil, nil, nil, nil))
	s := selectionFor(t, first)
	if s.Matches(changedPrevious) || s.Matches(foreign) {
		t.Fatal("reference ignored canonical contents or environment")
	}
	for _, tc := range []struct {
		selection      BaselineSelection
		previous, next Baseline
	}{
		{BaselineSelection{}, first, second}, {s, Baseline{}, second}, {s, first, Baseline{}}, {s, changedPrevious, second}, {s, first, first},
		{s, first, lateral}, {s, first, reused}, {s, first, foreign}, {s, first, changedCells}, {s, first, changedTime}, {s, first, changedPairs}, {s, first, reordered},
		{selectionFor(t, third), third, second},
	} {
		got, err := tc.selection.Advance(tc.previous, tc.next)
		if err == nil || got.EnvironmentID() != "" || got.BaselineID() != "" {
			t.Fatal("invalid selection transition admitted")
		}
	}
	body, _ := s.Encode()
	restarted, err := DecodeBaselineSelection(body, s.EnvironmentID(), s.InstallationID())
	if err != nil {
		t.Fatal(err)
	}
	got, err := restarted.Advance(first, third)
	if err != nil || !got.Matches(third) {
		t.Fatal("restart lost cumulative transition", err)
	}
}
