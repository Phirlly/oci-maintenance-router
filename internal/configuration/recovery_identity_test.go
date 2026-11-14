package configuration

import (
	"bytes"
	"os"
	"testing"
)

func TestRecoveryControlsPreserveOriginalRoutingAndEnvironment(t *testing.T) {
	original := decoded(t, settings(t))
	want, err := os.ReadFile("testdata/routing-canonical.json")
	if err != nil {
		t.Fatal(err)
	}
	location := LedgerLocation{Namespace: "example", Bucket: "ledger"}
	env, _ := original.EnvironmentID(location)
	for _, kind := range []string{"RETRY_BATCH", "APPROVE_REVISION", "CLOSE_BATCH", "CLOSE_REVISION"} {
		v := recoverySettings(t, kind)
		c := decoded(t, v)
		got, _ := c.canonicalRouting()
		actual, _ := c.EnvironmentID(location)
		if !bytes.Equal(got, bytes.TrimSpace(want)) || generation(t, c) != generation(t, original) || actual != env {
			t.Fatal("recovery changed identity")
		}
		v["routes"].([]any)[0].(map[string]any)["ownerValue"] = "different"
		if generation(t, decoded(t, v)) == generation(t, original) {
			t.Fatal("real routing edit lost identity change")
		}
	}
}
