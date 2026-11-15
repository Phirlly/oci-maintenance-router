package deployment

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"oci-maintenance-router/internal/reconciliation"
)

func TestOperationalWorkUsesOriginalBudgetAndCancellation(t *testing.T) {
	for _, kind := range []string{"caller-deadline", "work-cap", "canceled", "reserve-exhausted"} {
		t.Run(kind, func(t *testing.T) {
			h, s, objects, _ := operationalFixture(t)
			p, failure := loadPinned(operationalContext(t), s, objects)
			if failure.Status != "" {
				t.Fatal(failure)
			}
			started := time.Now().Add(-time.Second)
			deadline := time.Now().Add(time.Minute)
			if kind == "work-cap" {
				deadline = time.Now().Add(10 * time.Minute)
			}
			if kind == "reserve-exhausted" {
				deadline = time.Now().Add(5 * time.Second)
			}
			ctx, cancel := context.WithDeadline(context.Background(), deadline)
			defer cancel()
			if kind == "canceled" {
				cancel()
			}
			called := false
			h.compose = func(Settings, Store, pinnedInputs, string) (operations, error) {
				return operations{initialize: func(ctx context.Context, _ *reconciliation.Lease) error {
					called = true
					got, ok := ctx.Deadline()
					want := deadline.Add(-10 * time.Second)
					if kind == "work-cap" {
						want = started.Add(240 * time.Second)
					}
					if !ok || !got.Equal(want) {
						t.Fatalf("work budget extended: got=%v want=%v", got, want)
					}
					return errors.New("stop before preparation")
				}}, nil
			}
			got := h.runOperational(ctx, started, s, objects, p, command{operation: "deployment-check"})
			if got.Status != "not-ready" || called != (kind == "caller-deadline" || kind == "work-cap") || got.ScanCompleted {
				t.Fatalf("unexpected lifetime result: %+v; called=%v", got, called)
			}
			if !called && (got.Code != "deadline" || len(objects.objects) != 0) {
				t.Fatal("expired work acquired ownership")
			}
		})
	}
}

func TestCancellationReleasesOwnershipWithoutRevivingWork(t *testing.T) {
	for _, cancelWork := range []bool{false, true} {
		t.Run(map[bool]string{false: "operation-error", true: "canceled"}[cancelWork], func(t *testing.T) {
			h, s, objects, _ := operationalFixture(t)
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()
			originalDeadline, _ := ctx.Deadline()
			cleaned := false
			objects.before = func(cleanup context.Context, method, _ string) error {
				if method == "REPLACE" {
					deadline, ok := cleanup.Deadline()
					if cleanup.Err() != nil || !ok || deadline.After(originalDeadline) || time.Until(deadline) > 10*time.Second {
						t.Fatal("cleanup lost original bounds")
					}
					cleaned = true
				}
				return nil
			}
			h.compose = func(Settings, Store, pinnedInputs, string) (operations, error) {
				return operations{initialize: func(work context.Context, lease *reconciliation.Lease) error {
					if cancelWork {
						cancel()
						if lease.Check(context.Background(), time.Second) == nil || work.Err() == nil {
							t.Fatal("cancellation revived ownership")
						}
					}
					return errors.New("private failure")
				}}, nil
			}
			response := h.run(withInvocationHeaders(ctx, nil), strings.NewReader(invocation(s)))
			got := response.operational
			want := "operation_failed"
			if cancelWork {
				want = "canceled"
			}
			if got == nil || got.Status != "not-ready" || got.Code != want || got.CleanupCode != "" || !cleaned || objects.version != 2 {
				t.Fatalf("cleanup failed: %+v cleaned=%v", got, cleaned)
			}
		})
	}
}
