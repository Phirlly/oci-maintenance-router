package configuration

import (
	"bytes"
	"testing"
)

func TestLiveControlsPreserveOriginalRoutingProjection(t *testing.T) {
	original := decoded(t, settings(t))
	want, _ := original.canonicalRouting()
	location := LedgerLocation{Namespace: "example", Bucket: "ledger"}
	env, _ := original.EnvironmentID(location)
	for _, mode := range []string{"PREVIEW", "PAUSED", "TEST", "LIVE"} {
		v := liveSettings(t)
		v["mode"] = mode
		c := decoded(t, v)
		got, _ := c.canonicalRouting()
		actual, _ := c.EnvironmentID(location)
		if !bytes.Equal(got, want) || generation(t, c) != generation(t, original) || actual != env {
			t.Fatal("schema/mode changed routing or environment")
		}
		v["routes"].([]any)[0].(map[string]any)["ownerValue"] = "changed"
		if generation(t, decoded(t, v)) == generation(t, original) {
			t.Fatal("real route edit lost generation change")
		}
	}
}
