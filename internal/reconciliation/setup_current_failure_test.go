package reconciliation

import (
	"context"
	"errors"
	"oci-maintenance-router/internal/ocistorage"
	"testing"
)

func TestSetupCurrentUnknownWritesRequireExactReadback(t *testing.T) {
	for _, kind := range []string{"lost-without-write", "conflicting-selection", "concurrent-exact", "canceled-after-write"} {
		t.Run(kind, func(t *testing.T) {
			s, objects, r := setupCurrentStoreFixture(t)
			ctx, cancel := context.WithCancel(leaseBudget(t))
			defer cancel()
			objects.before = func(_ context.Context, c discoveryCall) error {
				if c.method != "CREATE" || c.name != s.name() {
					return nil
				}
				switch kind {
				case "lost-without-write":
					return errors.New("private write failure")
				case "conflicting-selection", "concurrent-exact":
					other := r
					if kind == "conflicting-selection" {
						d, o := setupCurrentIntents(t, 2)
						var err error
						other, err = newSetupCurrent(2, d, o)
						if err != nil {
							t.Fatal(err)
						}
					}
					body, _ := other.encode()
					objects.seed(s.name(), body)
					return &ocistorage.Error{Code: ocistorage.Conflict, Status: 412}
				}
				return nil
			}
			objects.after = func(_ context.Context, c discoveryCall) error {
				if kind == "canceled-after-write" && c.method == "CREATE" && c.name == s.name() {
					cancel()
				}
				return nil
			}
			got, err := s.create(ctx, r)
			if kind == "concurrent-exact" {
				if err != nil || !sameSetupCurrent(got.record, r) {
					t.Fatal("exact concurrent selection not confirmed", err)
				}
			} else {
				var detail *SetupError
				if !errors.As(err, &detail) || got.record.data != nil {
					t.Fatal("unknown/different write granted handle", err)
				}
				if (kind == "lost-without-write" || kind == "canceled-after-write") && !detail.WriteUncertain {
					t.Fatal("unknown write became definite", err)
				}
				if kind != "canceled-after-write" && errors.Is(err, context.Canceled) {
					t.Fatal("storage failure misreported as cancellation")
				}
			}
			if preparationCalls(objects, "CREATE", s.name()) != 1 {
				t.Fatal("write retried")
			}
		})
	}
}

func TestSetupCurrentGuardStopsMutationAfterHistoryRead(t *testing.T) {
	f := setupRuntimeFixtureFor(t)
	f.objects.after = func(_ context.Context, c discoveryCall) error {
		if c.method == "HISTORY" && c.name == "control/setup-current/installation-1.json" {
			f.objects.seed("configuration/router.json", []byte("changed"))
		}
		return nil
	}
	_, err := f.coordinator.Run(leaseBudget(t), f.lease)
	var detail *SetupError
	if !errors.As(err, &detail) || detail.Code != "configuration_changed" || detail.WriteUncertain || preparationCalls(f.objects, "CREATE", "control/setup-current/installation-1.json") != 0 || f.diagnosticSends != 0 {
		t.Fatal("changed configuration wrote current selection", err)
	}
}

func TestSetupCurrentStoreRequiresBoundedAuthorityAndExactHandle(t *testing.T) {
	s, objects, r := setupCurrentStoreFixture(t)
	if _, _, err := s.load(context.Background()); err == nil {
		t.Fatal("unbounded load accepted")
	}
	if _, err := s.create(context.Background(), r); err == nil {
		t.Fatal("unbounded create accepted")
	}
	first, err := s.create(leaseBudget(t), r)
	if err != nil {
		t.Fatal(err)
	}
	before := preparationCalls(objects, "CREATE", s.name())
	if again, err := s.create(leaseBudget(t), r); err != nil || again.etag != first.etag || preparationCalls(objects, "CREATE", s.name()) != before {
		t.Fatal("initial repeat wrote again", err)
	}
	d, o := setupCurrentIntents(t, 2)
	next, err := first.record.selectAttempt(2, d, o)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.save(context.Background(), first, next); err == nil {
		t.Fatal("unbounded update accepted")
	}
	foreign, _ := newSetupCurrentStore(objects, s.environment, s.installation)
	if _, err = foreign.save(leaseBudget(t), first, next); err == nil {
		t.Fatal("foreign handle accepted")
	}
	ctx, cancel := context.WithCancel(leaseBudget(t))
	cancel()
	if _, _, err = s.load(ctx); !errors.Is(err, context.Canceled) {
		t.Fatal("canceled read accepted", err)
	}
	objects.calls = nil
	if _, _, err = s.read(ctx); !errors.Is(err, context.Canceled) {
		t.Fatal("cancellation before read dispatch was ignored", err)
	}
	if _, err = s.write(ctx, "", r); !errors.Is(err, context.Canceled) {
		t.Fatal("cancellation before write dispatch was ignored", err)
	}
	if len(objects.calls) != 0 {
		t.Fatal("canceled dispatch reached backend")
	}
}
