package configuration

import (
	"strings"
	"testing"
)

func TestLiveControlsRetainStrictJSONAndRejectLegacyAuthority(t *testing.T) {
	wire := string(encoded(t, liveSettings(t)))
	for _, version := range []string{"8", "6.0", "6e0", `"6"`} {
		wantError(t, []byte(strings.Replace(wire, `"schemaVersion":6`, `"schemaVersion":`+version, 1)), "schemaVersion", "unsupported")
	}
	for _, field := range []string{"activeBaselineId", "replay", "activationId", "baselineId", "ready", "private-input"} {
		v := liveSettings(t)
		v[field] = "private-value"
		body := encoded(t, v)
		wantError(t, body, "configuration", "unknown_field")
		_, err := Decode(body, catalog())
		if strings.Contains(err.Error(), "private") {
			t.Fatal("input escaped error")
		}
	}
	duplicate := strings.Replace(wire, `"mode":"LIVE"`, `"mode":"LIVE","mode":"TEST"`, 1)
	wantError(t, []byte(duplicate), "configuration", "duplicate_key")
	alias := strings.Replace(wire, `"mode":"LIVE"`, `"Mode":"LIVE"`, 1)
	wantError(t, []byte(alias), "mode", "required")
	for _, version := range []int{2, 3, 4, 5} {
		v := preparationSettings(t)
		v["schemaVersion"] = version
		v["mode"] = "LIVE"
		wantError(t, encoded(t, v), "mode", "invalid")
	}
}
