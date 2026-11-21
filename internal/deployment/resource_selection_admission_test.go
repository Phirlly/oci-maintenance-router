package deployment

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestResourceSelectionHandlerRefusesBeforeOperations(t *testing.T) {
	for _, mode := range []string{"PREVIEW", "PAUSED", "TEST", "LIVE"} {
		t.Run(mode, func(t *testing.T) {
			h, s, store := handlerFixture(t)
			read := func(name string) []byte {
				b, err := os.ReadFile("../configuration/testdata/" + name + ".json")
				if err != nil {
					t.Fatal(err)
				}
				return b
			}
			var v map[string]any
			if err := json.Unmarshal(read("selected"), &v); err != nil {
				t.Fatal(err)
			}
			v["mode"] = mode
			v["operatorChannel"] = map[string]any{"subscriptionId": "ocid1.onssubscription.oc1.iad.operator", "endpoint": "operator@example.invalid"}
			if mode == "TEST" {
				v["setup"] = map[string]any{"action": "SEND_TEST", "attempt": 1, "secretId": "ocid1.vaultsecret.oc1.iad.test", "nonProduction": true}
			}
			store.config, _ = json.Marshal(v)
			store.catalog = read("selected-catalog")
			s.ConfigurationSHA256, s.CatalogSHA256 = digest(store.config), digest(store.catalog)
			bootstrap, _ := json.Marshal(s)
			h.getenv = func(string) string { return string(bootstrap) }
			h.compose = func(Settings, Store, pinnedInputs, string) (operations, error) {
				t.Fatal("unsupported configuration composed services")
				return operations{}, nil
			}
			h.metrics = func(Settings, Store, string) (metricPublisher, error) {
				t.Fatal("unsupported configuration composed metrics")
				return nil, nil
			}
			p, failure := loadPinned(testContext(t), s, store)
			if failure.Status != "" || p.config.SchemaVersion != 9 {
				t.Fatal("fixture rejected before runtime admission", failure)
			}
			for _, body := range []string{invocation(s), `{"operation":"reconcile"}`} {
				result := h.run(testContext(t), strings.NewReader(body))
				if result.legacy.Code != "configuration_unsupported" || result.legacy.SendingEnabled || result.operational != nil || store.creates != 0 || len(store.markers) != 0 {
					t.Fatalf("unsupported format performed work: %+v", result)
				}
			}
		})
	}
}
