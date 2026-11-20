package reconciliation

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestSetupOperatorReadinessRejectsMismatchedRecordAndFutureAcceptance(t *testing.T) {
	for _, kind := range []string{"identity", "accepted-after-confirmation", "reserve"} {
		t.Run(kind, func(t *testing.T) {
			f := setupConfirmedRuntime(t)
			v := setupReadinessFor(t, f)
			s, h := selectedSetupFor(t, f)
			name := "control/operator-challenges/installation-1/" + h.record.data.OperatorRequestID + ".json"
			if kind == "reserve" {
				f.objects.after = func(_ context.Context, c discoveryCall) error {
					if c.name == name {
						f.lease.manager.now = func() time.Time { return f.lease.deadline.Add(-time.Second) }
					}
					return nil
				}
			} else {
				old := f.result.Operator.Record().Snapshot()
				intent := old.Intent
				if kind == "identity" {
					intent.EndpointDigest = strings.Repeat("a", 64)
				}
				changed, err := NewOperatorChallenge(intent, OperatorClaim{old.OwnerToken, old.ClaimToken, old.StartedAt, old.ExpiresAt})
				if err != nil {
					t.Fatal(err)
				}
				changed, err = changed.Finish(OperatorClaimRef{old.OwnerToken, old.ClaimToken}, 200, "altered-message", time.Now().Add(time.Minute))
				if err != nil {
					t.Fatal(err)
				}
				body, _ := changed.Encode()
				f.objects.seed(name, body)
				if kind == "accepted-after-confirmation" {
					cs, _ := NewSetupConfirmationStore(f.objects, s.environment, s.installation)
					c, _, err := cs.load(leaseBudget(t), h.record.data.Confirmation.ID)
					if err != nil {
						t.Fatal(err)
					}
					c.data.OperatorSHA256 = operatorDigest(string(body))
					cb, _ := c.Encode()
					f.objects.seed(cs.name(c.ID()), cb)
					h.record.data.Confirmation = &setupConfirmationRef{c.ID(), diagnosticDigest(string(cb))}
					hb, _ := h.record.encode()
					f.objects.seed(s.name(), hb)
				}
			}
			v.read = func(context.Context) (OperatorChannel, error) {
				t.Fatal("invalid record reached channel lookup")
				return OperatorChannel{}, nil
			}
			if got, err := v.verifyOperator(leaseBudget(t), f.lease); err == nil || got.Record().Snapshot().State != "" {
				t.Fatal("invalid operator evidence returned proof", err)
			}
		})
	}
}

func TestSetupConfirmationRechecksControlsAfterBothReceipts(t *testing.T) {
	f := setupRuntimeFixtureFor(t)
	var err error
	f.result, err = f.coordinator.Run(leaseBudget(t), f.lease)
	if err != nil {
		t.Fatal(err)
	}
	f.configure(t, "CONFIRM", 1)
	observed := false
	f.coordinator.read = func(context.Context) (OperatorChannel, error) { observed = true; return operatorChannelFixture(), nil }
	f.objects.after = func(_ context.Context, c discoveryCall) error {
		if observed && c.name == "configuration/catalog.json" {
			f.objects.seed("configuration/router.json", []byte("changed after receipt verification"))
		}
		return nil
	}
	got, err := f.coordinator.Run(leaseBudget(t), f.lease)
	if err == nil || got.Confirmation.Confirmation.ID() != "" || got.Confirmation.Diagnostic.Record().Snapshot().State != DiagnosticAccepted || got.Confirmation.Operator.Record().Snapshot().State != OperatorAccepted {
		t.Fatal("changed controls attached confirmation", err)
	}
}
