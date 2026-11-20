package reconciliation

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestSetupConfirmationDiscardsLateReadAndNeverRecreatesIt(t *testing.T) {
	s, objects, r := setupStoreFixture(t)
	body, _ := r.Encode()
	objects.seed(s.name(r.ID()), body)
	ctx, cancel := context.WithCancel(leaseBudget(t))
	defer cancel()
	objects.after = func(_ context.Context, c discoveryCall) error {
		if c.name == s.name(r.ID()) && c.method == "GET" {
			cancel()
		}
		return nil
	}
	got, err := s.Create(ctx, r)
	if got.ID() != "" || !errors.Is(err, context.Canceled) || preparationCalls(objects, "CREATE", "") != 0 {
		t.Fatal("late read accepted", err)
	}
}

func TestSetupConfirmationConcurrentCreatePreservesOneImmutableRecord(t *testing.T) {
	s, objects, r := setupStoreFixture(t)
	var wg sync.WaitGroup
	for range 6 {
		wg.Go(func() {
			got, err := s.Create(leaseBudget(t), r)
			if err != nil || got.ID() != r.ID() {
				t.Error("concurrent confirmation failed", err)
			}
		})
	}
	wg.Wait()
	obj := objects.objects[s.name(r.ID())]
	got, err := DecodeSetupConfirmation(obj.Body, r.data.EnvironmentID, r.data.InstallationID, r.ID())
	if err != nil || got.confirmedAt() != r.confirmedAt() {
		t.Fatal("concurrent proposals replaced provenance", err)
	}
	if preparationCalls(objects, "REPLACE", "") != 0 {
		t.Fatal("immutable confirmation overwritten")
	}
}

func TestSetupConfirmationReserveStopsStorageAccess(t *testing.T) {
	_, objects, _ := setupStoreFixture(t)
	failure := &SetupConfirmationError{Code: "insufficient_time"}
	storage := setupConfirmationStorage{objects, func() error { return failure }, func() error { return failure }}
	if _, err := storage.Get(leaseBudget(t), "unused"); err != failure {
		t.Fatal(err)
	}
	if _, err := storage.HistoryIsEmpty(leaseBudget(t), "unused"); err != failure {
		t.Fatal(err)
	}
	if _, err := storage.Create(leaseBudget(t), "unused", nil); err == nil || err.Error() != "setup confirmation dispatch blocked" {
		t.Fatal(err)
	}
	if len(objects.calls) != 0 {
		t.Fatal("exhausted work accessed storage")
	}
}

func TestSetupConfirmationRejectsChangesBetweenVerificationAndPersistence(t *testing.T) {
	for _, kind := range []string{"after-channel-verification", "clock-after-write"} {
		t.Run(kind, func(t *testing.T) {
			c, f := setupCoordinatorFixture(t)
			if kind == "after-channel-verification" {
				channelReturned := false
				c.operator.read = func(context.Context) (OperatorChannel, error) {
					channelReturned = true
					return c.operator.input.desired, nil
				}
				f.objects.after = func(_ context.Context, call discoveryCall) error {
					if channelReturned && call.name == "configuration/catalog.json" {
						channelReturned = false
						f.objects.seed("configuration/router.json", []byte("changed between checks"))
					}
					return nil
				}
			} else {
				at := time.Now().Add(time.Second)
				f.lease.manager.now = func() time.Time { return at }
				f.objects.after = func(_ context.Context, call discoveryCall) error {
					if call.method == "CREATE" && strings.HasPrefix(call.name, "control/setup-confirmations/") {
						at = at.Add(-500 * time.Millisecond)
					}
					return nil
				}
			}
			got, err := c.Confirm(leaseBudget(t), f.lease)
			var detail *SetupConfirmationError
			want := "configuration_changed"
			if kind == "clock-after-write" {
				want = "invalid_time"
			}
			if !errors.As(err, &detail) || detail.Code != want || got.Confirmation.ID() != "" {
				t.Fatal("late change reported success", err)
			}
			if kind == "after-channel-verification" && preparationCalls(f.objects, "CREATE", "") != 0 {
				t.Fatal("changed configuration permitted write")
			}
		})
	}
}
