package deployment

import (
	"encoding/json"
	"testing"
)

func TestCheckRejectsPreparationSchemaBeforeMarker(t *testing.T) {
	for _, initialize := range []bool{false, true} {
		s, store := fixture(t)
		var v map[string]any
		if err := json.Unmarshal(store.config, &v); err != nil {
			t.Fatal(err)
		}
		v["schemaVersion"] = 2
		v["mode"] = "PREVIEW"
		v["installationId"] = "installation-1"
		v["preparation"] = map[string]any{"initialize": initialize}
		var err error
		store.config, err = json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		s.ConfigurationSHA256 = digestFixture(store.config)
		got := check(testContext(t), s, store, testAttemptID)
		if got.Status != "not-ready" || got.Code != "configuration_unsupported" || got.SendingEnabled || store.creates != 0 || len(store.markers) != 0 {
			t.Fatalf("old installer accepted unsupported preparation: %+v; writes=%d", got, store.creates)
		}
	}
}
