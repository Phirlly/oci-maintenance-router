package configuration

import (
	"bytes"
	"testing"
)

func TestPreparationSchemaPreservesExactRoutingAndEnvironmentIdentity(t *testing.T) {
	v1 := decoded(t, settings(t))
	v2 := decoded(t, preparationSettings(t))
	a, _ := v1.canonicalRouting()
	b, _ := v2.canonicalRouting()
	if !bytes.Equal(a, b) || generation(t, v2) != "2cc11f0431bdd77c4232e1274c22d93f4e57f46d7fa35e352201a3296022aee7" {
		t.Fatal("outer schema changed routing meaning")
	}
	loc := LedgerLocation{Namespace: "example", Bucket: "ledger"}
	e1, err := v1.EnvironmentID(loc)
	if err != nil {
		t.Fatal(err)
	}
	e2, err := v2.EnvironmentID(loc)
	if err != nil || e2 != e1 {
		t.Fatal("outer schema changed environment")
	}
	for _, change := range []func(map[string]any){
		func(v map[string]any) { v["installationId"] = "other-installation" },
		func(v map[string]any) { v["mode"] = "PAUSED" },
		func(v map[string]any) { v["preparation"] = map[string]any{"initialize": false} },
	} {
		v := preparationSettings(t)
		change(v)
		c := decoded(t, v)
		if generation(t, c) != generation(t, v1) {
			t.Fatal("preparation control changed routing")
		}
		e, _ := c.EnvironmentID(loc)
		if e != e1 {
			t.Fatal("preparation control changed environment")
		}
	}
	v := preparationSettings(t)
	v["pagerDutyRegion"] = "EU"
	if generation(t, decoded(t, v)) == generation(t, v1) {
		t.Fatal("real routing edit lost generation change")
	}
}
