package reconciliation

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"testing"
)

func TestPreparationIntentIndependentGolden(t *testing.T) {
	scope := discoveryScope{regions: []string{"us-ashburn-1"}, compartments: []string{baselineCompartment}}
	b := preparationIntent(strings.Repeat("a", 64), "", scope, []string{"Oracle HeatWave MySQL"})
	want := `["baseline-intent-v1","aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",null,["us-ashburn-1"],["ocid1.compartment.oc1..example"],["Oracle HeatWave MySQL"]]`
	if string(b) != want || fmt.Sprintf("p1-%x", sha256.Sum256(b)) != "p1-5d7ea712a80a3b1c3b52c0c275e480c240dad94d6fcafc5817625d358c4c0238" {
		t.Fatal("intent differs from independently computed JSON/SHA256 golden")
	}
}

func TestPreparationIntentIncludesOnlyOwnedScopeServicesAndPredecessor(t *testing.T) {
	p, s, l := preparationFixture(t, nil, func(c map[string]any) {
		c["compartmentIds"] = []string{baselineOtherCompartment, baselineCompartment}
		c["mysqlServices"] = []string{"MySQL", "Oracle HeatWave MySQL"}
	})
	h, _ := NewPreparedHistory(l.manager.environmentID, "installation-1")
	want := p.targetID(h)
	for _, change := range []func(map[string]any){
		func(c map[string]any) { c["environmentName"] = "renamed" },
		func(c map[string]any) { c["ownership"] = map[string]any{"kind": "freeform", "key": "team"} },
		func(c map[string]any) { c["mode"] = "PAUSED"; c["preparation"] = map[string]any{"initialize": false} },
		func(c map[string]any) { c["operatorTopicId"] = "ocid1.onstopic.oc1.iad.other" },
	} {
		cfg := preparationConfig(t, func(c map[string]any) {
			c["compartmentIds"] = []string{baselineCompartment, baselineOtherCompartment}
			c["mysqlServices"] = []string{"Oracle HeatWave MySQL", "MySQL"}
		}, change)
		other := preparationFor(t, s, cfg, nil)
		if other.targetID(h) != want {
			t.Fatal("unrelated controls or array order changed intent")
		}
	}
	for _, change := range []func(map[string]any){func(c map[string]any) { c["compartmentIds"] = []string{baselineCompartment} }, func(c map[string]any) { c["mysqlServices"] = []string{"MySQL"} }, func(c map[string]any) {
		c["regionMap"] = map[string]string{"us-ashburn-1": "iad", "us-phoenix-1": "phx"}
	}} {
		cfg := preparationConfig(t, func(c map[string]any) {
			c["compartmentIds"] = []string{baselineCompartment, baselineOtherCompartment}
			c["mysqlServices"] = []string{"Oracle HeatWave MySQL", "MySQL"}
		}, change)
		if preparationFor(t, s, cfg, nil).targetID(h) == want {
			t.Fatal("scope/service change did not change intent")
		}
	}
	first := buildBaseline(t, "previous", baselineResult(t, nil, nil, nil, nil))
	next, err := h.Advance(Baseline{}, first)
	if err != nil {
		t.Fatal(err)
	}
	if p.targetID(next) == want {
		t.Fatal("predecessor digest omitted from identity")
	}
	other, _ := NewPreparedHistory(strings.Repeat("b", 64), "installation-1")
	if p.targetID(other) == want {
		t.Fatal("environment omitted from identity")
	}
}
