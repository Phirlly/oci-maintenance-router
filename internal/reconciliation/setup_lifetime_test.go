package reconciliation

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestSetupReadinessKeepsOriginalBoundedLifetime(t *testing.T) {
	f := setupConfirmedRuntime(t)
	v := setupReadinessFor(t, f)
	ctx, cancel := context.WithTimeout(leaseBudget(t), 40*time.Second)
	defer cancel()
	deadline, _ := ctx.Deadline()
	var reads []context.Context
	f.objects.before = func(c context.Context, call discoveryCall) error {
		reads = append(reads, c)
		d, ok := c.Deadline()
		if !ok || d.After(deadline) || call.name != workerLeaseName && d.After(time.Now().Add(discoveryReserve)) {
			t.Fatal("read escaped original bound", call.name)
		}
		return nil
	}
	if err := v.verify(ctx, f.lease, strings.Repeat("b", 64)); err != nil {
		t.Fatal(err)
	}
	for _, c := range reads {
		if c.Err() == nil {
			t.Fatal("read context retained")
		}
	}
}

func TestSetupReadinessCancellationNeverReturnsProof(t *testing.T) {
	for _, stage := range []string{"initial", "head", "confirmation", "diagnostic", "operator", "final"} {
		t.Run(stage, func(t *testing.T) {
			f := setupConfirmedRuntime(t)
			v := setupReadinessFor(t, f)
			ctx, cancel := context.WithCancel(leaseBudget(t))
			defer cancel()
			if stage == "initial" {
				cancel()
			}
			catalogReads := 0
			f.objects.after = func(c context.Context, call discoveryCall) error {
				if call.name == "configuration/catalog.json" {
					catalogReads++
				}
				match := stage == "head" && strings.HasPrefix(call.name, "control/setup-current/") || stage == "confirmation" && strings.HasPrefix(call.name, "control/setup-confirmations/") || stage == "diagnostic" && strings.HasPrefix(call.name, "control/diagnostics/") || stage == "operator" && strings.HasPrefix(call.name, "control/operator-challenges/") || stage == "final" && catalogReads == 2
				if match {
					cancel()
				}
				return nil
			}
			if err := v.verify(ctx, f.lease, strings.Repeat("b", 64)); !errors.Is(err, context.Canceled) {
				t.Fatal("canceled verification returned success or lost classification", err)
			}
		})
	}
}

func TestSetupCoordinatorRejectsInvalidLifetimeAndCurrentConfiguration(t *testing.T) {
	for _, kind := range []string{"no-deadline", "canceled", "expired", "lease-released", "configuration", "catalog", "reserve"} {
		t.Run(kind, func(t *testing.T) {
			f := setupRuntimeFixtureFor(t)
			ctx := leaseBudget(t)
			switch kind {
			case "no-deadline":
				ctx = context.Background()
			case "canceled":
				c, cancel := context.WithCancel(ctx)
				cancel()
				ctx = c
			case "expired":
				c, cancel := context.WithDeadline(ctx, time.Now().Add(-time.Second))
				defer cancel()
				ctx = c
			case "lease-released":
				if err := f.lease.Release(ctx); err != nil {
					t.Fatal(err)
				}
			case "configuration":
				f.objects.seed("configuration/router.json", []byte("changed"))
			case "catalog":
				f.objects.seed("configuration/catalog.json", []byte("changed"))
			case "reserve":
				f.lease.manager.now = func() time.Time { return f.lease.deadline.Add(-time.Second) }
			}
			f.objects.calls = nil
			if _, err := f.coordinator.Run(ctx, f.lease); err == nil {
				t.Fatal("invalid authority accepted")
			}
			if f.diagnosticSends != 0 || f.operatorSends != 0 {
				t.Fatal("invalid authority sent")
			}
			for _, call := range f.objects.calls {
				if call.method != "GET" {
					t.Fatal("invalid authority changed setup", call)
				}
			}
		})
	}
}
