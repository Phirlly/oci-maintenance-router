package reconciliation

import "testing"

func TestPreparedHistoryRejectsRewrittenOrSkippedHistory(t *testing.T) {
	_, _, first, second := preparedStoreFixture(t)
	empty, _ := NewPreparedHistory(first.EnvironmentID(), "installation-1")
	h := advancePrepared(t, empty, Baseline{}, first)
	third, err := second.Extend("third", baselineResult(t, nil, nil, nil, []string{"us-phoenix-1"}))
	if err != nil {
		t.Fatal(err)
	}
	changedFirst := buildBaseline(t, first.ID(), baselineResult(t, nil, nil, []string{baselineOtherCompartment}, nil))
	rewrittenBody := alteredBaseline(t, second, func(d *baselineData) { d.Layers[0].Cells[0].Region = "us-phoenix-1" })
	rewritten, err := DecodeBaseline(rewrittenBody, first.EnvironmentID(), second.ID())
	if err != nil {
		t.Fatal("fixture is not valid baseline", err)
	}
	reusedBody := alteredBaseline(t, second, func(d *baselineData) { d.BaselineID = first.ID() })
	reused, err := DecodeBaseline(reusedBody, first.EnvironmentID(), first.ID())
	if err != nil {
		t.Fatal(err)
	}
	foreignBody := alteredBaseline(t, first, func(d *baselineData) {
		d.EnvironmentID = "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
	})
	foreign, err := DecodeBaseline(foreignBody, "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", first.ID())
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		head           PreparedHistory
		previous, next Baseline
	}{
		{PreparedHistory{}, Baseline{}, first}, {empty, Baseline{}, Baseline{}}, {empty, first, first}, {empty, Baseline{}, second}, {empty, Baseline{}, foreign},
		{h, Baseline{}, second}, {h, changedFirst, second}, {h, first, first}, {h, first, reused}, {h, first, rewritten}, {h, first, third},
	} {
		got, err := tt.head.Advance(tt.previous, tt.next)
		if err == nil || got.EnvironmentID() != "" {
			t.Fatal("invalid cumulative transition admitted")
		}
	}
	body, _ := h.Encode()
	restarted, err := DecodePreparedHistory(body, h.EnvironmentID(), h.InstallationID())
	if err != nil {
		t.Fatal(err)
	}
	if next := advancePrepared(t, restarted, first, second); next.BaselineID() != second.ID() {
		t.Fatal("restart lost predecessor identity")
	}
}
