package reconciliation

import (
	"fmt"
	"strings"
	"testing"
)

func TestPreparationScopeCapacityFailsWithoutDiscoveryOrHeadAdvancement(t *testing.T) {
	for _, kind := range []string{"product", "bytes"} {
		p, s, l := preparationFixture(t, nil, func(c map[string]any) {
			c["regionMap"] = map[string]string{"us-ashburn-1": "iad", "us-phoenix-1": "phx"}
			if kind == "product" {
				compartments := make([]string, 20000)
				for i := range compartments {
					compartments[i] = fmt.Sprintf("ocid1.compartment.oc1..c%05d", i)
				}
				c["compartmentIds"] = compartments
			} else {
				c["compartmentIds"] = []string{baselineCompartment + strings.Repeat("a", 530000)}
			}
		})
		if err := p.Initialize(leaseBudget(t), l); err != nil {
			t.Fatal(err)
		}
		b, err := p.Prepare(leaseBudget(t), l)
		preparationFailed(t, b, err, "too_large")
		if preparationCalls(s, "LIST", "") != 0 || preparationHead(t, s, l).BaselineID() != "" {
			t.Fatal("oversized scope attempted collection or advanced history")
		}
	}
}
