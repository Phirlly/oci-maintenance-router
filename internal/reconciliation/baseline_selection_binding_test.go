package reconciliation

import "testing"

func TestBaselineSelectionStoreBindsExactImmediatePredecessor(t *testing.T) {
	s, objects, backend, first, second := selectionStoreFixture(t)
	ctx := leaseBudget(t)
	a := selectionSaved(t, s, first)
	b, err := s.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	next := advanceSelection(t, a.Selection(), first, second)
	body, _ := next.Encode()
	decoded, err := DecodeBaselineSelection(body, first.EnvironmentID(), "installation-1")
	if err != nil {
		t.Fatal(err)
	}
	third, err := second.Extend("third", baselineResult(t, nil, nil, nil, []string{"us-phoenix-1"}))
	if err != nil {
		t.Fatal(err)
	}
	twoSteps := advanceSelection(t, next, second, third)
	other := selectionStoreFor(t, backend, first)
	before := len(objects.calls)
	for _, tc := range []struct {
		store *BaselineSelectionStore
		old   StoredBaselineSelection
		next  BaselineSelection
	}{
		{s, b, next}, {s, a, decoded}, {s, a, twoSteps}, {s, a, a.Selection()}, {s, a, BaselineSelection{}}, {s, StoredBaselineSelection{}, next}, {other, a, next}, {nil, a, next}, {&BaselineSelectionStore{}, a, next},
	} {
		got, err := tc.store.Save(ctx, tc.old, tc.next)
		selectionStoredFailure(t, got, err, "invalid_request", 0, false)
	}
	if len(objects.calls) != before {
		t.Fatal("unbound transition dispatched")
	}
	saved, err := s.Save(ctx, a, next)
	if err != nil {
		t.Fatal(err)
	}
	confirmed, err := s.Save(ctx, a, next)
	if err != nil || confirmed.Selection().BaselineDigest() != saved.Selection().BaselineDigest() {
		t.Fatal("exact prior write not confirmed", err)
	}
	final := advanceSelection(t, confirmed.Selection(), second, third)
	if _, err := s.Save(ctx, confirmed, final); err != nil {
		t.Fatal("confirmation returned unusable version", err)
	}
}
