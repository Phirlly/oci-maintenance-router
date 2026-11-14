package reconciliation

import "testing"

func TestPreparedHistoryStoreBindsExactImmediatePredecessorAndStore(t *testing.T) {
	s, objects, first, second := preparedStoreFixture(t)
	ctx := leaseBudget(t)
	a, err := s.Create(ctx)
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	proposal := advancePrepared(t, a.History(), Baseline{}, first)
	body, _ := proposal.Encode()
	decoded, err := DecodePreparedHistory(body, first.EnvironmentID(), "installation-1")
	if err != nil {
		t.Fatal(err)
	}
	twoSteps := advancePrepared(t, proposal, first, second)
	other := preparedStoreFor(t, objects, first.EnvironmentID(), "installation-1")
	before := len(objects.calls)
	for _, tt := range []struct {
		store *PreparedHistoryStore
		old   StoredPreparedHistory
		next  PreparedHistory
	}{
		{s, b, proposal}, {s, a, decoded}, {s, a, twoSteps}, {s, a, PreparedHistory{}}, {s, StoredPreparedHistory{}, proposal}, {other, a, proposal}, {nil, a, proposal}, {&PreparedHistoryStore{}, a, proposal},
	} {
		got, err := tt.store.Save(ctx, tt.old, tt.next)
		preparedStoredFailure(t, got, err, "invalid_request", 0, false)
	}
	if len(objects.calls) != before {
		t.Fatal("unbound proposal dispatched")
	}
	saved, err := s.Save(ctx, a, proposal)
	if err != nil {
		t.Fatal(err)
	}
	// A repeated exact old proposal can confirm its prior write, but a fresh
	// handle cannot be attached to an unrelated old proposal.
	confirmed, err := s.Save(ctx, a, proposal)
	if err != nil || confirmed.History().BaselineDigest() != saved.History().BaselineDigest() {
		t.Fatal("exact repeat not confirmed", err)
	}
	next := advancePrepared(t, confirmed.History(), first, second)
	if _, err := s.Save(ctx, confirmed, next); err != nil {
		t.Fatal("confirmation did not return current version", err)
	}
}

func TestPreparedHistoryCreateConfirmationRetainsProgress(t *testing.T) {
	s, objects, first, _ := preparedStoreFixture(t)
	ctx := leaseBudget(t)
	empty, err := s.Create(ctx)
	if err != nil {
		t.Fatal(err)
	}
	prepared := advancePrepared(t, empty.History(), Baseline{}, first)
	if _, err := s.Save(ctx, empty, prepared); err != nil {
		t.Fatal(err)
	}
	before := objects.version
	got, err := s.Create(ctx)
	if err != nil || got.History().BaselineDigest() != prepared.BaselineDigest() || objects.version != before {
		t.Fatal("initialization retry reset prepared history", err)
	}
	foreign := preparedStoreFor(t, objects, first.EnvironmentID(), "other-installation")
	got, err = foreign.Create(ctx)
	preparedStoredFailure(t, got, err, "invalid_record", 200, false)
	if objects.version != before {
		t.Fatal("foreign create overwrote history")
	}
}
