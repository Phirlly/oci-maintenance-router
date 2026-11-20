package deployment

import (
	"encoding/json"
	"testing"
)

func retainedSetupIDs(environment string) (string, string) {
	d, _ := json.Marshal([]any{"setup-diagnostic-v1", environment, "synthetic-installation", 1})
	o, _ := json.Marshal([]any{"setup-operator-v1", environment, "synthetic-installation", 1})
	return "sd1-" + digestFixture(d), "so1-" + digestFixture(o)
}

// Seed accepted records through their real domain/store APIs to exercise retained
// result projection. This does not claim fresh delivery or paired confirmation.
func TestSetupHandlerReusesRetainedOutcomesWithoutCredentialAccess(t *testing.T) {
	for _, tc := range []struct {
		name     string
		d, o     int
		accepted bool
	}{
		{"accepted", 202, 202, true}, {"diagnostic-active", 0, 202, false}, {"diagnostic-review", 400, 202, false}, {"operator-active", 202, 0, false}, {"operator-review", 202, 400, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h, s, objects, env, principal := retainedTestFixture(t, true)
			s = configureOperational(t, h, s, objects, setupConfiguration("TEST", "SEND_TEST"))
			d, o := retainedSetupIDs(env)
			dc := retainDiagnostic(t, s, objects, env, tc.d, d)
			oc := retainOperator(t, objects, env, tc.o, o)
			for i := 0; i < 2; i++ {
				got := invokeOperational(t, h, invocation(s))
				if (got.Status == "operational-verified") != tc.accepted || got.Diagnostic == nil || got.Diagnostic.Dispatched || receiptVerified(t, got.Diagnostic) || receiptVerified(t, got.Operator) {
					t.Fatal("retained setup outcome misreported", got)
				}
				if tc.d != 202 && got.Operator != nil {
					t.Fatal("operator ran after diagnostic failure")
				}
				if tc.d == 202 && (got.Operator == nil || got.Operator.Dispatched) {
					t.Fatal("operator outcome lost or resent")
				}
				requireReceiptPrivacy(t, got, dc, oc)
			}
			if principal.reads != 0 {
				t.Fatal("retained setup read credentials")
			}
		})
	}
}

func TestSetupHandlerConfirmsWithoutFallingBackToSender(t *testing.T) {
	h, s, objects, env, principal := retainedTestFixture(t, true)
	s = configureOperational(t, h, s, objects, setupConfiguration("TEST", "SEND_TEST"))
	d, o := retainedSetupIDs(env)
	dc := retainDiagnostic(t, s, objects, env, 202, d)
	oc := retainOperator(t, objects, env, 202, o)
	if got := invokeOperational(t, h, invocation(s)); got.Status != "operational-verified" {
		t.Fatal(got)
	}
	s = configureOperational(t, h, s, objects, func(v map[string]any) {
		setupConfiguration("TEST", "CONFIRM")(v)
		q := v["setup"].(map[string]any)
		q["pagerDutyCode"], q["operatorCode"] = dc, oc
	})
	before := objects.version
	got := invokeOperational(t, h, invocation(s))
	// Actual channel lookup reaches the intentionally failing signer; it cannot
	// prove receipt and must not dispatch or attach confirmation.
	if got.Status != "not-ready" || got.Code != "channel_read" || !receiptVerified(t, got.Diagnostic) || receiptVerified(t, got.Operator) || principal.reads == 0 {
		t.Fatal("confirmation skipped actual channel lookup", got)
	}
	if objects.version != before+2 {
		t.Fatal("failed confirmation changed more than the lease")
	}
	requireReceiptPrivacy(t, got, dc, oc)
}
