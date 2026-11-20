package reconciliation

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestSetupConfirmationRechecksProofAfterRouteAndChannelChanges(t *testing.T) {
	c, f := setupCoordinatorFixture(t)
	first, err := c.Confirm(leaseBudget(t), f.lease)
	if err != nil {
		t.Fatal(err)
	}
	before, _ := first.Confirmation.Encode()
	setupConfigure(t, f, func(v map[string]any) { v["routes"].([]any)[0].(map[string]any)["ownerValue"] = "changed-team" })
	if got, err := c.Confirm(leaseBudget(t), f.lease); err == nil || got.Confirmation.ID() != "" {
		t.Fatal("stale pins reused confirmation")
	}
	current, err := NewSetupConfirmationCoordinator(f.config, []byte(preparationCatalog), candidateLocation(), strings.Repeat("b", 64), f.objects, operatorClient(t))
	if err != nil {
		t.Fatal(err)
	}
	current.operator.read = func(context.Context) (OperatorChannel, error) { return current.operator.input.desired, nil }
	got, err := current.Confirm(leaseBudget(t), f.lease)
	after, _ := got.Confirmation.Encode()
	if err != nil || !bytes.Equal(before, after) || preparationCalls(f.objects, "CREATE", "") != 1 {
		t.Fatal("routing edit rewrote independent confirmation", err)
	}
	current.operator.read = func(context.Context) (OperatorChannel, error) { return OperatorChannel{}, nil }
	if got, err := current.Confirm(leaseBudget(t), f.lease); err == nil || got.Confirmation.ID() != "" {
		t.Fatal("saved confirmation bypassed changed channel")
	}
}

func TestSetupConfirmationHonorsLifetimeAndFinalGuard(t *testing.T) {
	for _, kind := range []string{"nil", "unbounded", "canceled", "expired", "reserve", "post-write", "post-write-cancel", "history-cancel"} {
		t.Run(kind, func(t *testing.T) {
			c, f := setupCoordinatorFixture(t)
			ctx, cancel := context.WithCancel(leaseBudget(t))
			defer cancel()
			switch kind {
			case "nil":
				ctx = nil
			case "unbounded":
				ctx = context.Background()
			case "canceled":
				cancel()
			case "expired":
				var stop context.CancelFunc
				ctx, stop = context.WithDeadline(ctx, time.Now().Add(-time.Second))
				defer stop()
			case "reserve":
				at := f.lease.deadline.Add(-time.Second)
				f.lease.manager.now = func() time.Time { return at }
			case "history-cancel":
				f.objects.history = func(context.Context, string) (bool, error) { cancel(); return true, nil }
			case "post-write", "post-write-cancel":
				f.objects.after = func(_ context.Context, call discoveryCall) error {
					if call.method == "CREATE" {
						if kind == "post-write" {
							f.objects.seed("configuration/router.json", []byte("changed"))
						} else {
							cancel()
						}
					}
					return nil
				}
			}
			got, err := c.Confirm(ctx, f.lease)
			if err == nil || got.Confirmation.ID() != "" {
				t.Fatal("invalid lifetime/final guard returned saved success")
			}
			if strings.Contains(kind, "cancel") && !errors.Is(err, context.Canceled) {
				t.Fatal("cancellation lost", err)
			}
			if kind == "expired" && !errors.Is(err, context.DeadlineExceeded) {
				t.Fatal("deadline lost", err)
			}
			if !strings.HasPrefix(kind, "post-write") && preparationCalls(f.objects, "CREATE", "") != 0 {
				t.Fatal("invalid guard wrote confirmation")
			}
		})
	}
}

func TestSetupConfirmationConstructionRequiresPairedExplicitTest(t *testing.T) {
	c, f := setupCoordinatorFixture(t)
	for _, kind := range []string{"live", "missing-operator-code", "missing-diagnostic-code", "missing-channel", "invalid-release"} {
		t.Run(kind, func(t *testing.T) {
			setupConfigure(t, f, func(v map[string]any) {
				switch kind {
				case "live":
					v["mode"] = "LIVE"
				case "missing-operator-code":
					delete(v["operatorTest"].(map[string]any), "receiptCode")
				case "missing-diagnostic-code":
					delete(v["diagnostic"].(map[string]any), "receiptCode")
				}
			})
			channel, release := operatorClient(t), strings.Repeat("b", 64)
			if kind == "missing-channel" {
				channel = nil
			}
			if kind == "invalid-release" {
				release = "invalid"
			}
			if got, err := NewSetupConfirmationCoordinator(f.config, []byte(preparationCatalog), candidateLocation(), release, f.objects, channel); err == nil || got != nil {
				t.Fatal("partial confirmation authority constructed")
			}
		})
	}
	for _, coordinator := range []*SetupConfirmationCoordinator{nil, {}} {
		if _, err := coordinator.Confirm(leaseBudget(t), f.lease); err == nil {
			t.Fatal("invalid coordinator accepted")
		}
	}
	if _, err := c.Confirm(leaseBudget(t), nil); err == nil {
		t.Fatal("missing lease accepted")
	}
	_, _, foreign := preparationFixture(t, nil, func(v map[string]any) { v["tenancyId"] = "ocid1.tenancy.oc1..foreign" })
	if _, err := c.Confirm(leaseBudget(t), foreign); err == nil {
		t.Fatal("foreign lease accepted")
	}
}
