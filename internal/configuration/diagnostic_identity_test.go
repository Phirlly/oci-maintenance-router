package configuration

import (
	"bytes"
	"testing"
)

func TestDiagnosticControlsPreserveRoutingAndEnvironmentIdentity(t *testing.T) {
	original := decoded(t, settings(t))
	want, _ := original.canonicalRouting()
	location := LedgerLocation{Namespace: "example", Bucket: "ledger"}
	env, _ := original.EnvironmentID(location)
	for _, change := range []func(map[string]any){
		func(map[string]any) {}, func(v map[string]any) { v["mode"] = "PREVIEW"; delete(v, "diagnostic") }, func(v map[string]any) { v["mode"] = "PAUSED" },
		func(v map[string]any) { v["diagnostic"].(map[string]any)["requestId"] = "next" }, func(v map[string]any) {
			v["diagnostic"].(map[string]any)["secretId"] = "ocid1.vaultsecret.oc1.iad.other"
		},
		func(v map[string]any) {
			v["installationId"] = "other"
			v["preparation"] = map[string]any{"initialize": false}
		},
	} {
		v := diagnosticSettings(t)
		change(v)
		c := decoded(t, v)
		got, _ := c.canonicalRouting()
		actual, _ := c.EnvironmentID(location)
		if !bytes.Equal(got, want) || generation(t, c) != "2cc11f0431bdd77c4232e1274c22d93f4e57f46d7fa35e352201a3296022aee7" || actual != env {
			t.Fatal("test controls changed routing/environment")
		}
		if len(c.Destinations) != 1 || c.Destinations[0].SecretID != "ocid1.vaultsecret.oc1.iad.example" {
			t.Fatal("test secret entered production destinations")
		}
	}
	v := diagnosticSettings(t)
	v["pagerDutyRegion"] = "EU"
	if generation(t, decoded(t, v)) == generation(t, original) {
		t.Fatal("real routing edit lost its generation change")
	}
}
