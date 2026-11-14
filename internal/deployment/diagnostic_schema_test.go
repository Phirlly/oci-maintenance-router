package deployment

import (
	"encoding/json"
	"testing"
)

func TestCheckRejectsDiagnosticSchemaBeforeMarker(t *testing.T) {
	for _, mode := range []string{"PREVIEW", "PAUSED", "TEST"} {
		s, store := fixture(t)
		var v map[string]any
		if err := json.Unmarshal(store.config, &v); err != nil {
			t.Fatal(err)
		}
		v["schemaVersion"] = 3
		v["mode"] = mode
		v["installationId"] = "installation-1"
		v["preparation"] = map[string]any{"initialize": true}
		v["diagnostic"] = map[string]any{"requestId": "test-1", "secretId": "ocid1.vaultsecret.oc1.iad.test", "nonProduction": true}
		var err error
		store.config, err = json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		s.ConfigurationSHA256 = digestFixture(store.config)
		got := check(testContext(t), s, store, testAttemptID)
		if got.Status != "not-ready" || got.Code != "configuration_unsupported" || got.SendingEnabled || store.creates != 0 || len(store.markers) != 0 {
			t.Fatalf("legacy installer misreported new control: %+v", got)
		}
	}
}
