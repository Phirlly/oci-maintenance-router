package configuration

import (
	"bytes"
	"testing"
)

func TestOperatorControlsPreserveRoutingAndEnvironment(t *testing.T) {
	original := decoded(t, settings(t))
	want, _ := original.canonicalRouting()
	location := LedgerLocation{Namespace: "example", Bucket: "ledger"}
	env, _ := original.EnvironmentID(location)
	for _, change := range []func(map[string]any){
		func(map[string]any) {},
		func(v map[string]any) { delete(v, "operatorTest") },
		func(v map[string]any) { v["mode"] = "PAUSED" },
		func(v map[string]any) { v["operatorTest"].(map[string]any)["requestId"] = "next" },
		func(v map[string]any) {
			v["operatorTest"].(map[string]any)["subscriptionId"] = "ocid1.onssubscription.oc1.iad.replaced"
		},
		func(v map[string]any) { v["operatorTest"].(map[string]any)["endpoint"] = "other@example.invalid" },
		func(v map[string]any) { v["operatorTest"].(map[string]any)["receiptCode"] = "" },
		func(v map[string]any) { v["operatorTopicId"] = "ocid1.onstopic.oc1.iad.replaced" },
	} {
		v := operatorSettings(t)
		change(v)
		c := decoded(t, v)
		got, _ := c.canonicalRouting()
		actual, _ := c.EnvironmentID(location)
		if !bytes.Equal(got, want) || generation(t, c) != generation(t, original) || actual != env {
			t.Fatal("operator controls changed routing or environment identity")
		}
		v["routes"].([]any)[0].(map[string]any)["ownerValue"] = "different"
		if generation(t, decoded(t, v)) == generation(t, original) {
			t.Fatal("real route edit lost generation change")
		}
	}
}
