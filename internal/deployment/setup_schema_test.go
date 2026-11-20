package deployment

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func setupConfiguration(mode, action string) func(map[string]any) {
	return func(v map[string]any) {
		v["schemaVersion"], v["mode"] = 8, mode
		delete(v, "diagnostic")
		delete(v, "operatorTest")
		v["operatorChannel"] = map[string]any{"subscriptionId": "ocid1.onssubscription.oc1.iad.operator", "endpoint": "operator@example.invalid"}
		s := map[string]any{"action": action}
		if action != "NONE" {
			s["attempt"] = 1
		}
		if action == "SEND_TEST" {
			s["secretId"], s["nonProduction"] = "ocid1.vaultsecret.oc1.iad.diagnostic", true
		}
		if action == "CONFIRM" {
			s["pagerDutyCode"], s["operatorCode"] = "OMR-"+strings.Repeat("a", 32), "OMR-OP-"+strings.Repeat("b", 32)
		}
		v["setup"] = s
	}
}

func TestSetupHandlerUsesSavedSelectionBeforeCredentials(t *testing.T) {
	for _, action := range []string{"SEND_TEST", "CONFIRM"} {
		t.Run(action, func(t *testing.T) {
			h, s, objects, _, principal := retainedTestFixture(t, true)
			s = configureOperational(t, h, s, objects, setupConfiguration("TEST", action))
			objects.before = func(_ context.Context, method, name string) error {
				if strings.HasPrefix(name, "control/setup-current/") {
					return errors.New("private selection failure")
				}
				return nil
			}
			got := invokeOperational(t, h, invocation(s))
			if got.ResponseVersion != 2 || got.Status != "not-ready" || got.Code != "read_failed" || principal.reads != 0 {
				t.Fatalf("setup not guarded: %+v credential reads=%d", got, principal.reads)
			}
			requireReceiptPrivacy(t, got, "OMR-"+strings.Repeat("a", 32), "OMR-OP-"+strings.Repeat("b", 32))
		})
	}
}

func TestSetupHandlerPausedDoesNoSetupWork(t *testing.T) {
	h, s, objects, _, principal := retainedTestFixture(t, true)
	s = configureOperational(t, h, s, objects, setupConfiguration("PAUSED", "NONE"))
	got := invokeOperational(t, h, invocation(s))
	if got.Status != "operational-verified" || got.ResponseVersion != 2 || got.Diagnostic != nil || got.Operator != nil || principal.reads != 0 {
		t.Fatalf("paused setup changed behavior: %+v", got)
	}
}
