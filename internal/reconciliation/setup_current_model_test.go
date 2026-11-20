package reconciliation

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"
)

func setupCurrentIntents(t *testing.T, n int64) (DiagnosticIntent, OperatorIntent) {
	t.Helper()
	d, o := diagnosticIntentFixture(), operatorIntentFixture(t)
	d.RequestID, o.RequestID = setupRequestIDs(d.EnvironmentID, d.InstallationID, n)
	return d, o
}

func TestSetupCurrentRequestNamespacesPreserveLegacySeparation(t *testing.T) {
	env, install := strings.Repeat("a", 64), "installation-1"
	d, o := setupRequestIDs(env, install, 1)
	for _, tc := range []struct{ domain, prefix, got string }{
		{"setup-diagnostic-v1", "sd1-", d}, {"setup-operator-v1", "so1-", o},
	} {
		wire, _ := json.Marshal([]any{tc.domain, env, install, int64(1)})
		hash := sha256.Sum256(wire)
		if tc.got != tc.prefix+hex.EncodeToString(hash[:]) {
			t.Fatal("combined identity differs from specified tuple")
		}
	}
	d2, o2 := setupRequestIDs(env, install, 2)
	if d == o || d == d2 || o == o2 || strings.HasPrefix(d, "d1-") || strings.HasPrefix(o, "o1-") {
		t.Fatal("attempt or legacy namespace collision")
	}
}

func TestSetupCurrentAdvancesOnlyExplicitHigherIntent(t *testing.T) {
	d, o := setupCurrentIntents(t, 1)
	first, err := newSetupCurrent(1, d, o)
	if err != nil || first.data == nil {
		t.Fatal("first explicit selection rejected", err)
	}
	before, _ := first.encode()
	same, err := first.selectAttempt(1, d, o)
	got, _ := same.encode()
	if err != nil || !bytes.Equal(before, got) || same.data != first.data {
		t.Fatal("unchanged intent changed selection", err)
	}
	changed := d
	changed.SecretID = "ocid1.vaultsecret.oc1.iad.changed"
	if _, err = first.selectAttempt(1, changed, o); err == nil {
		t.Fatal("same number changed its intent")
	}
	d2, o2 := setupCurrentIntents(t, 2)
	next, err := first.selectAttempt(2, d2, o2)
	if err != nil || next.data.Attempt != 2 || next.parent != first.data || next.data.Confirmation != nil {
		t.Fatal("higher attempt did not bind predecessor", err)
	}
	if _, err = next.selectAttempt(1, d, o); err == nil {
		t.Fatal("lower attempt restored old authority")
	}
	for _, n := range []int64{0, -1, 2147483648} {
		if _, err = newSetupCurrent(n, d, o); err == nil {
			t.Fatal("invalid attempt admitted")
		}
	}
}

func TestSetupCurrentRejectsForeignAndLegacyIntents(t *testing.T) {
	d, o := setupCurrentIntents(t, 1)
	for _, kind := range []string{"environment", "installation", "legacy-diagnostic", "legacy-operator", "invalid-intent"} {
		diag, operator := d, o
		switch kind {
		case "environment":
			operator.EnvironmentID = strings.Repeat("b", 64)
		case "installation":
			operator.InstallationID = "another"
		case "legacy-diagnostic":
			diag.RequestID = "d1-legacy"
		case "legacy-operator":
			operator.RequestID = "o1-legacy"
		case "invalid-intent":
			diag.NonProduction = false
		}
		if _, err := newSetupCurrent(1, diag, operator); err == nil {
			t.Fatal("unrelated intent joined selection", kind)
		}
	}
}
