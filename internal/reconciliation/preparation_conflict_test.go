package reconciliation

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"

	"oci-maintenance-router/internal/ocistorage"
)

func TestPreparationInitializationConflictPreservesProgressedHead(t *testing.T) {
	p, s, l := preparationFixture(t, nil)
	first := buildBaseline(t, "preexisting", baselineResult(t, nil, nil, nil, nil))
	h, _ := NewPreparedHistory(l.manager.environmentID, "installation-1")
	h, err := h.Advance(Baseline{}, first)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := h.Encode()
	s.before = func(_ context.Context, c discoveryCall) error {
		if c.method == "CREATE" && c.name == preparedHistoryName {
			s.seed(preparedHistoryName, body)
		}
		return nil
	}
	if err := p.Initialize(leaseBudget(t), l); err != nil {
		t.Fatal(err)
	}
	if preparationHead(t, s, l).BaselineID() != first.ID() || preparationCalls(s, "CREATE", preparedHistoryName) != 1 {
		t.Fatal("initialization conflict reset progressed state")
	}
}

func TestPreparationConflictingHeadCannotMergeOrOverwriteWinner(t *testing.T) {
	p, s, l := preparationFixture(t, nil)
	if err := p.Initialize(leaseBudget(t), l); err != nil {
		t.Fatal(err)
	}
	winner := buildBaseline(t, "other-winner", baselineResult(t, nil, nil, nil, nil))
	h := preparationHead(t, s, l)
	h, err := h.Advance(Baseline{}, winner)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := h.Encode()
	s.before = func(_ context.Context, c discoveryCall) error {
		if c.method == "REPLACE" && c.name == preparedHistoryName {
			s.seed(preparedHistoryName, body)
		}
		return nil
	}
	b, err := p.Prepare(leaseBudget(t), l)
	preparationFailed(t, b, err, "identity_conflict")
	if preparationHead(t, s, l).BaselineID() != winner.ID() || preparationCalls(s, "REPLACE", preparedHistoryName) != 1 {
		t.Fatal("competing head was overwritten or merged")
	}
}

func TestPreparationConcurrentEquivalentRunsConfirmOneTarget(t *testing.T) {
	p, s, l := preparationFixture(t, nil)
	if err := p.Initialize(leaseBudget(t), l); err != nil {
		t.Fatal(err)
	}
	var reads atomic.Int32
	both := make(chan struct{})
	s.before = func(ctx context.Context, c discoveryCall) error {
		if c.method == "GET" && strings.HasPrefix(c.name, "baselines/") {
			n := reads.Add(1)
			if n <= 2 {
				if n == 2 {
					close(both)
				}
				select {
				case <-both:
					return &ocistorage.Error{Code: ocistorage.Service, Status: 404}
				case <-ctx.Done():
					return ctx.Err()
				}
			}
		}
		return nil
	}
	type outcome struct {
		b   Baseline
		err error
	}
	results := make(chan outcome, 2)
	for range 2 {
		go func() { b, err := p.Prepare(leaseBudget(t), l); results <- outcome{b, err} }()
	}
	a, b := <-results, <-results
	if a.err != nil || b.err != nil || a.b.ID() == "" || a.b.ID() != b.b.ID() {
		t.Fatal("equivalent concurrent runs did not converge", a.err, b.err)
	}
	if preparationHead(t, s, l).BaselineID() != a.b.ID() {
		t.Fatal("confirmed result differs from head")
	}
}
