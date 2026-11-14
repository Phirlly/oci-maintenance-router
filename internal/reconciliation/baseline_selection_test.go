package reconciliation

import (
	"os"
	"testing"
)

func selectionFor(t *testing.T, b Baseline) BaselineSelection {
	t.Helper()
	s, err := NewBaselineSelection(b.EnvironmentID(), "installation-1", b)
	if err != nil {
		t.Fatal("valid initial selection rejected", err)
	}
	return s
}

func TestBaselineSelectionCanonicalReferenceAndRestart(t *testing.T) {
	body, err := os.ReadFile("testdata/empty-baseline.json")
	if err != nil {
		t.Fatal(err)
	}
	env := "c081fc9b5129be4e85c6039722858f2454d79fcd9219504f473d53dffd712378"
	b, err := DecodeBaseline(body, env, "initial")
	if err != nil {
		t.Fatal(err)
	}
	s := selectionFor(t, b)
	// Digest independently calculated from the existing synthetic canonical fixture.
	want := `{"schemaVersion":1,"environmentId":"` + env + `","installationId":"installation-1","selectedBaseline":{"baselineId":"initial","contentSha256":"f335e5c382f1c7e7e5850626907e6278d37b68149349f79bb4b374264d95efa6"}}`
	wire, err := s.Encode()
	if err != nil || string(wire) != want || s.EnvironmentID() != env || s.InstallationID() != "installation-1" || s.BaselineID() != b.ID() || !s.Matches(b) || s.Matches(Baseline{}) {
		t.Fatal("canonical selection or exact identity changed", err)
	}
	decoded, err := DecodeBaselineSelection(wire, env, "installation-1")
	clear(wire)
	again, encodeErr := decoded.Encode()
	if err != nil || encodeErr != nil || string(again) != want || !decoded.Matches(b) {
		t.Fatal("restart lost owned selection", err, encodeErr)
	}
	clear(again)
	if owned, _ := decoded.Encode(); string(owned) != want {
		t.Fatal("encoding exposed state")
	}
}

func TestBaselineSelectionAcceptsMultiplePreparedLayers(t *testing.T) {
	_, _, first, second := preparedStoreFixture(t)
	third, err := second.Extend("third", baselineResult(t, nil, nil, nil, []string{"us-phoenix-1"}))
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range []Baseline{first, second, third} {
		if s := selectionFor(t, b); !s.Matches(b) || s.BaselineID() != b.ID() {
			t.Fatal("initial selection rejected cumulative baseline")
		}
	}
	s := selectionFor(t, first)
	for _, b := range []Baseline{second, third} {
		next, err := s.Advance(first, b)
		if err != nil || !next.Matches(b) || !s.Matches(first) {
			t.Fatal("cumulative selection mutated history or rejected appended layers", err)
		}
		body, _ := next.Encode()
		restarted, err := DecodeBaselineSelection(body, s.EnvironmentID(), s.InstallationID())
		if err != nil || !restarted.Matches(b) {
			t.Fatal("restart changed reference", err)
		}
	}
}
