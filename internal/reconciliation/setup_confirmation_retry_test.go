package reconciliation

import (
	"context"
	"oci-maintenance-router/internal/ocistorage"
	"testing"
)

func TestSetupConfirmationFailedAttachmentRetriesWithoutSending(t *testing.T) {
	f := setupRuntimeFixtureFor(t)
	var err error
	f.result, err = f.coordinator.Run(leaseBudget(t), f.lease)
	if err != nil {
		t.Fatal(err)
	}
	f.configure(t, "CONFIRM", 1)
	f.objects.before = func(_ context.Context, c discoveryCall) error {
		if c.method == "REPLACE" && c.name == "control/setup-current/installation-1.json" {
			return &ocistorage.Error{Code: ocistorage.Service, Status: 403}
		}
		return nil
	}
	got, err := f.coordinator.Run(leaseBudget(t), f.lease)
	if err == nil || got.Confirmation.Confirmation.ID() != "" || got.Confirmation.Diagnostic.Record().Snapshot().State != DiagnosticAccepted || got.Confirmation.Operator.Record().Snapshot().State != OperatorAccepted {
		t.Fatal("failed attachment granted completion or lost verified receipts", err)
	}
	_, h := selectedSetupFor(t, f)
	if h.record.data.Confirmation != nil {
		t.Fatal("failed attachment saved authority")
	}
	f.objects.before = nil
	for i := 0; i < 2; i++ {
		got, err = f.coordinator.Run(leaseBudget(t), f.lease)
		if err != nil || got.Confirmation.Confirmation.ID() == "" {
			t.Fatal("attachment retry failed", err)
		}
	}
	if f.diagnosticSends != 1 || f.operatorSends != 1 {
		t.Fatal("confirmation retry resent tests")
	}
}

func TestSetupConfirmationRefusesWrongAttemptAndCodes(t *testing.T) {
	for _, kind := range []string{"attempt", "diagnostic-code", "operator-code", "missing-record"} {
		t.Run(kind, func(t *testing.T) {
			f := setupRuntimeFixtureFor(t)
			var err error
			f.result, err = f.coordinator.Run(leaseBudget(t), f.lease)
			if err != nil {
				t.Fatal(err)
			}
			n := int64(1)
			if kind == "attempt" {
				n = 2
			}
			f.configure(t, "CONFIRM", n, func(v map[string]any) {
				s := v["setup"].(map[string]any)
				if kind == "diagnostic-code" {
					s["pagerDutyCode"] = "OMR-00000000000000000000000000000000"
				}
				if kind == "operator-code" {
					s["operatorCode"] = "OMR-OP-00000000000000000000000000000000"
				}
			})
			if kind == "missing-record" {
				delete(f.objects.objects, "control/setup-current/installation-1.json")
			}
			got, err := f.coordinator.Run(leaseBudget(t), f.lease)
			if err == nil || got.Confirmation.Confirmation.ID() != "" || f.diagnosticSends != 1 || f.operatorSends != 1 {
				t.Fatal("invalid confirmation accepted or sent", err)
			}
		})
	}
}
