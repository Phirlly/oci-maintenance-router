package reconciliation

import (
	"context"
	"errors"
	"strings"
	"testing"

	"oci-maintenance-router/internal/ocistorage"
)

func setupCoordinatorFixture(t *testing.T) (*SetupConfirmationCoordinator, *liveSelectorFixture) {
	t.Helper()
	f := liveSelectorFixtureFor(t)
	setupConfigure(t, f)
	c, err := NewSetupConfirmationCoordinator(f.config, []byte(preparationCatalog), candidateLocation(), strings.Repeat("b", 64), f.objects, operatorClient(t))
	if err != nil {
		t.Fatal(err)
	}
	c.operator.read = func(context.Context) (OperatorChannel, error) {
		f.channelReads.Add(1)
		return c.operator.input.desired, nil
	}
	f.objects.calls = nil
	return c, f
}

func setupConfigure(t *testing.T, f *liveSelectorFixture, changes ...func(map[string]any)) {
	t.Helper()
	f.config = liveSchemaConfig(t, "TEST", append([]func(map[string]any){func(v map[string]any) {
		v["schemaVersion"] = 7
		v["diagnostic"].(map[string]any)["receiptCode"] = f.diagnostic.Snapshot().ConfirmationCode
		v["operatorTest"].(map[string]any)["receiptCode"] = f.operator.Snapshot().ConfirmationCode
	}}, changes...)...)
	f.objects.seed("configuration/router.json", f.config)
}

func TestSetupConfirmationCoordinatorPersistsVerifiedPairWithoutResending(t *testing.T) {
	c, f := setupCoordinatorFixture(t)
	first, err := c.Confirm(leaseBudget(t), f.lease)
	if err != nil || first.Confirmation.ID() == "" || first.Diagnostic.Record().Snapshot().State != DiagnosticAccepted || first.Operator.Record().Snapshot().State != OperatorAccepted {
		t.Fatalf("paired confirmation failed: %v", err)
	}
	second, err := c.Confirm(leaseBudget(t), f.lease)
	if err != nil || second.Confirmation.ID() != first.Confirmation.ID() || preparationCalls(f.objects, "CREATE", "") != 1 || f.diagnosticSends.Load() != 1 || f.operatorSends.Load() != 1 {
		t.Fatal("repeat confirmation sent or rewrote evidence", err)
	}
	if f.channelReads.Load() != 2 {
		t.Fatal("repeat bypassed current operator channel")
	}
	for _, call := range f.objects.calls {
		if call.method != "GET" && call.method != "HISTORY" && !(call.method == "CREATE" && strings.HasPrefix(call.name, "control/setup-confirmations/")) {
			t.Fatal("confirmation mutated unrelated state", call.method, call.name)
		}
	}
}

func TestSetupConfirmationCoordinatorRejectsPartialAndChangedInputs(t *testing.T) {
	for _, kind := range []string{"diagnostic", "operator", "channel", "configuration", "history-change", "write-failure"} {
		t.Run(kind, func(t *testing.T) {
			c, f := setupCoordinatorFixture(t)
			switch kind {
			case "diagnostic":
				c.diagnostic.input.receiptCode = "wrong"
			case "operator":
				c.operator.input.receiptCode = "wrong"
			case "channel":
				c.operator.read = func(context.Context) (OperatorChannel, error) { return OperatorChannel{}, nil }
			case "configuration":
				f.objects.seed("configuration/router.json", []byte("changed"))
			case "history-change":
				f.objects.history = func(context.Context, string) (bool, error) {
					f.objects.seed("configuration/router.json", []byte("changed"))
					return true, nil
				}
			case "write-failure":
				f.objects.before = func(_ context.Context, call discoveryCall) error {
					if call.method == "CREATE" {
						return &ocistorage.Error{Code: ocistorage.Service, Status: 403}
					}
					return nil
				}
			}
			got, err := c.Confirm(leaseBudget(t), f.lease)
			if err == nil || got.Confirmation.ID() != "" {
				t.Fatal("incomplete confirmation succeeded")
			}
			if kind == "operator" || kind == "channel" {
				if got.Diagnostic.Record().Snapshot().State != DiagnosticAccepted || got.Operator.Record().Snapshot().State != "" {
					t.Fatal("partial verification lost")
				}
			}
			if kind == "write-failure" || kind == "history-change" {
				if got.Diagnostic.Record().Snapshot().State != DiagnosticAccepted || got.Operator.Record().Snapshot().State != OperatorAccepted {
					t.Fatal("verified channels lost after persistence failure")
				}
			}
			if kind != "write-failure" && preparationCalls(f.objects, "CREATE", "") != 0 {
				t.Fatal("failed guard wrote confirmation")
			}
			if kind == "history-change" {
				var failure *SetupConfirmationError
				if !errors.As(err, &failure) || failure.Code != "configuration_changed" || failure.WriteUncertain {
					t.Fatalf("pre-dispatch guard reported an uncertain write: %+v", err)
				}
			}
			if f.diagnosticSends.Load() != 1 || f.operatorSends.Load() != 1 {
				t.Fatal("failure resent test")
			}
		})
	}
}
