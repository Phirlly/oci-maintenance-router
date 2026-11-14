package reconciliation

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"oci-maintenance-router/internal/ocistorage"
)

func TestPreparationStorageRejectionsCannotBecomeInitializationOrCompletion(t *testing.T) {
	for _, stage := range []string{"initialize-write", "target-read", "baseline-write"} {
		p, s, l := preparationFixture(t, nil)
		if stage != "initialize-write" {
			if err := p.Initialize(leaseBudget(t), l); err != nil {
				t.Fatal(err)
			}
		}
		s.before = func(_ context.Context, c discoveryCall) error {
			match := stage == "initialize-write" && c.method == "CREATE" && c.name == preparedHistoryName || stage == "target-read" && c.method == "GET" && strings.HasPrefix(c.name, "baselines/") || stage == "baseline-write" && c.method == "CREATE" && strings.HasPrefix(c.name, "baselines/")
			if match {
				return &ocistorage.Error{Code: ocistorage.Service, Status: 403}
			}
			return nil
		}
		var b Baseline
		var err error
		if stage == "initialize-write" {
			err = p.Initialize(leaseBudget(t), l)
		} else {
			b, err = p.Prepare(leaseBudget(t), l)
		}
		code := "write_failed"
		if stage == "target-read" {
			code = "read_failed"
		}
		d := preparationFailed(t, b, err, code)
		if d.HTTPStatus != 403 || d.WriteUncertain {
			t.Fatal("definite rejection lost its meaning")
		}
		if preparationCalls(s, "REPLACE", preparedHistoryName) != 0 {
			t.Fatal("rejected operation advanced head")
		}
	}
}

func TestPreparationLateEmptyHistoryCannotCreate(t *testing.T) {
	p, s, l := preparationFixture(t, nil)
	ctx, cancel := context.WithCancel(leaseBudget(t))
	defer cancel()
	s.history = func(context.Context, string) (bool, error) { cancel(); return true, nil }
	err := p.Initialize(ctx, l)
	preparationFailed(t, Baseline{}, err, "canceled")
	if preparationCalls(s, "CREATE", preparedHistoryName) != 0 {
		t.Fatal("late empty response initialized")
	}
}

func TestPreparationExpiredContextDoesNotReviveWork(t *testing.T) {
	p, s, l := preparationFixture(t, nil)
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	before := len(s.calls)
	b, err := p.Prepare(ctx, l)
	preparationFailed(t, b, err, "deadline_exceeded")
	if !errors.Is(err, context.DeadlineExceeded) || len(s.calls) != before {
		t.Fatal("deadline error or no-I/O boundary changed")
	}
}

func TestPreparationRejectsRegressedCompletionClockDuringExpansion(t *testing.T) {
	p, s, l := preparationFixture(t, nil)
	if err := p.Initialize(leaseBudget(t), l); err != nil {
		t.Fatal(err)
	}
	first := prepareSuccessfully(t, p, l)
	p = preparationFor(t, s, preparationConfig(t, func(c map[string]any) { c["compartmentIds"] = []string{baselineCompartment, baselineOtherCompartment} }), nil)
	p.discovery.now = func() time.Time { return first.CompletedAt().Add(-time.Second) }
	b, err := p.Prepare(leaseBudget(t), l)
	preparationFailed(t, b, err, "invalid_time")
	if preparationHead(t, s, l).BaselineID() != first.ID() {
		t.Fatal("regressed clock replaced history")
	}
}
