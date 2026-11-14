package reconciliation

import (
	"context"
	"errors"
	"testing"

	"oci-maintenance-router/internal/ocistorage"
)

func TestPreparationInitializesOnceAndPreservesProgress(t *testing.T) {
	p, s, l := preparationFixture(t, nil)
	if err := p.Initialize(leaseBudget(t), l); err != nil {
		t.Fatal(err)
	}
	if preparationCalls(s, "CREATE", preparedHistoryName) != 1 || preparationCalls(s, "HISTORY", preparedHistoryName) != 1 {
		t.Fatal("initialization did not inspect history and conditionally create exactly once")
	}
	h := preparationHead(t, s, l)
	if h.BaselineID() != "" {
		t.Fatal("initialization selected a baseline")
	}
	b := prepareSuccessfully(t, p, l)
	before := preparationHead(t, s, l)
	if err := p.Initialize(leaseBudget(t), l); err != nil {
		t.Fatal(err)
	}
	after := preparationHead(t, s, l)
	if after.BaselineID() != b.ID() || after.BaselineDigest() != before.BaselineDigest() || preparationCalls(s, "CREATE", preparedHistoryName) != 1 || preparationCalls(s, "HISTORY", preparedHistoryName) != 1 {
		t.Fatal("reinitialization reset progress")
	}
}

func TestPreparationInitializationRequiresPinnedIntent(t *testing.T) {
	for _, change := range []func(map[string]any){func(c map[string]any) { c["mode"] = "PAUSED" }, func(c map[string]any) { c["preparation"] = map[string]any{"initialize": false} }} {
		p, s, l := preparationFixture(t, nil, change)
		before := len(s.calls)
		err := p.Initialize(leaseBudget(t), l)
		preparationFailed(t, Baseline{}, err, "intent_required")
		if len(s.calls) != before {
			t.Fatal("disabled initialization made I/O")
		}
	}
}

func TestPreparationNeverInitializesFromMissingHeadDuringPrepare(t *testing.T) {
	p, s, l := preparationFixture(t, nil)
	b, err := p.Prepare(leaseBudget(t), l)
	d := preparationFailed(t, b, err, "read_failed")
	if d.HTTPStatus != 404 {
		t.Fatal("missing head status lost")
	}
	if preparationCalls(s, "HISTORY", "") != 0 || preparationCalls(s, "CREATE", preparedHistoryName) != 0 {
		t.Fatal("ordinary preparation initialized missing history")
	}
}

func TestPreparationInitializationRefusesRetainedOrUnusableHistory(t *testing.T) {
	for _, tc := range []struct {
		empty bool
		err   error
		code  string
	}{{false, nil, "history_retained"}, {true, errors.New("private history"), "history_read"}, {true, &ocistorage.Error{Code: ocistorage.Service, Status: 403}, "history_read"}} {
		p, s, l := preparationFixture(t, nil)
		s.history = func(context.Context, string) (bool, error) { return tc.empty, tc.err }
		err := p.Initialize(leaseBudget(t), l)
		preparationFailed(t, Baseline{}, err, tc.code)
		if preparationCalls(s, "CREATE", preparedHistoryName) != 0 {
			t.Fatal("unproven empty history was initialized")
		}
	}
}
