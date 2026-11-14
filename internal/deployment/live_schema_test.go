package deployment

import (
	"encoding/json"
	"testing"
)

func TestCheckRejectsValidLIVEConfigurationSchemaBeforeMarker(t *testing.T) {
	for _, mode := range []string{"PREVIEW", "PAUSED", "TEST", "LIVE"} {
		s, store := fixture(t)
		var v map[string]any
		if err := json.Unmarshal(store.config, &v); err != nil {
			t.Fatal(err)
		}
		v["schemaVersion"], v["mode"] = 6, mode
		v["installationId"] = "installation-1"
		v["preparation"] = map[string]any{"initialize": true}
		v["diagnostic"] = map[string]any{"requestId": "test-1", "secretId": "ocid1.vaultsecret.oc1.iad.test", "nonProduction": true, "receiptCode": "OMR-0123456789abcdef0123456789abcdef"}
		v["operatorTest"] = map[string]any{"requestId": "operator-1", "subscriptionId": "ocid1.onssubscription.oc1.iad.operator", "endpoint": "Team@example.invalid", "receiptCode": "OMR-OP-0123456789abcdef0123456789abcdef"}
		var err error
		store.config, err = json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		s.ConfigurationSHA256 = digestFixture(store.config)
		got := check(testContext(t), s, store, testAttemptID)
		// configuration_unsupported is reached only after successful full decoding.
		if got.Status != "not-ready" || got.Code != "configuration_unsupported" || got.SendingEnabled || store.creates != 0 || len(store.markers) != 0 {
			t.Fatalf("legacy installer accepted operational schema or fixture invalid: %+v", got)
		}
	}
}
