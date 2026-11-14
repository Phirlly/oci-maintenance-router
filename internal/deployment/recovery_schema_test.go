package deployment

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestCheckRejectsRecoverySchemaBeforeMarker(t *testing.T) {
	for _, mode := range []string{"PREVIEW", "PAUSED", "TEST", "LIVE"} {
		for _, present := range []bool{false, true} {
			s, store := fixture(t)
			var v map[string]any
			if err := json.Unmarshal(store.config, &v); err != nil {
				t.Fatal(err)
			}
			v["schemaVersion"], v["mode"] = 7, mode
			v["installationId"] = "installation-1"
			v["preparation"] = map[string]any{"initialize": true}
			v["diagnostic"] = map[string]any{"requestId": "test-1", "secretId": "ocid1.vaultsecret.oc1.iad.test", "nonProduction": true, "receiptCode": "OMR-0123456789abcdef0123456789abcdef"}
			v["operatorTest"] = map[string]any{"requestId": "operator-1", "subscriptionId": "ocid1.onssubscription.oc1.iad.operator", "endpoint": "team@example.invalid", "receiptCode": "OMR-OP-0123456789abcdef0123456789abcdef"}
			if present {
				v["recovery"] = map[string]any{"requestId": "close-1", "noticeId": "ocid1.announcement.oc1..maintenance", "kind": "CLOSE_BATCH", "targetId": strings.Repeat("a", 64), "reviewFingerprint": strings.Repeat("b", 64), "routingGeneration": strings.Repeat("c", 64)}
			}
			var err error
			store.config, err = json.Marshal(v)
			if err != nil {
				t.Fatal(err)
			}
			s.ConfigurationSHA256 = digestFixture(store.config)
			got := check(testContext(t), s, store, testAttemptID)
			if got.Status != "not-ready" || got.Code != "configuration_unsupported" || got.SendingEnabled || store.creates != 0 || len(store.markers) != 0 {
				t.Fatalf("unsupported operational configuration accepted or invalid fixture: %+v", got)
			}
		}
	}
}
