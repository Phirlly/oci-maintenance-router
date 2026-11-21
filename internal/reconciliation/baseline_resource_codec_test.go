package reconciliation

import (
	"bytes"
	"strings"
	"testing"
)

// Independent canonical schema2 fixture; no real resource or cloud state.
const instanceBaselineWire = `{"schemaVersion":2,"environmentId":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","baselineId":"instance-initial","completedAt":"2026-11-13T12:00:00.000Z","layers":[{"completedAt":"2026-11-13T12:00:00.000Z","cells":[{"region":"us-ashburn-1","compartmentId":"ocid1.compartment.oc1..example","resourceType":"instance"}],"revisions":[]}]}`

func TestBaselineInstanceCanonicalRoundTrip(t *testing.T) {
	wire := []byte(instanceBaselineWire)
	b, err := DecodeBaseline(wire, strings.Repeat("a", 64), "instance-initial")
	if err != nil {
		t.Fatal("valid resource-qualified baseline rejected", err)
	}
	clear(wire)
	layers := b.Layers()
	layers[0].Cells[0].ResourceType = "changed"
	got, err := b.Encode()
	if err != nil || !bytes.Equal(got, []byte(instanceBaselineWire)) {
		t.Fatal("typed baseline canonical bytes changed", err)
	}
}

func TestBaselineRejectsAmbiguousResourceWire(t *testing.T) {
	for _, wire := range []string{
		strings.Replace(instanceBaselineWire, `"schemaVersion":2`, `"schemaVersion":1`, 1),
		strings.Replace(instanceBaselineWire, `"instance"`, `"volume"`, 1),
		strings.Replace(instanceBaselineWire, `"instance"`, `"mysqldbsystem"`, 1),
		strings.Replace(instanceBaselineWire, `,"resourceType":"instance"`, ``, 1),
		strings.Replace(instanceBaselineWire, `"resourceType":"instance"`, `"resourceType":"instance","resourceType":"instance"`, 1),
		strings.Replace(instanceBaselineWire, `"resourceType":"instance"`, `"ResourceType":"instance"`, 1),
		strings.Replace(instanceBaselineWire, `"resourceType":"instance"`, `"resourceType":null`, 1),
	} {
		b, err := DecodeBaseline([]byte(wire), strings.Repeat("a", 64), "instance-initial")
		if err == nil || b.ID() != "" {
			t.Fatal("ambiguous resource identity admitted")
		}
	}
}

func TestBaselineTypedCellsRejectDuplicatesAndNoncanonicalOrder(t *testing.T) {
	b := buildBaseline(t, "both", resourceBaselineResult(t, []string{"instance", "mysqldbsystem"}))
	cells := b.Layers()[0].Cells
	if len(cells) != 2 || cells[0].ResourceType != "instance" || cells[1].ResourceType != "" {
		t.Fatal("type ordering or legacy encoding changed")
	}
	for _, change := range []func(*baselineData){
		func(d *baselineData) { d.Layers[0].Cells[1] = d.Layers[0].Cells[0] },
		func(d *baselineData) { d.Layers = append(d.Layers, d.Layers[0]) },
		func(d *baselineData) {
			d.Layers[0].Cells[0], d.Layers[0].Cells[1] = d.Layers[0].Cells[1], d.Layers[0].Cells[0]
		},
	} {
		got, err := DecodeBaseline(alteredBaseline(t, b, change), b.EnvironmentID(), b.ID())
		if err == nil || got.ID() != "" {
			t.Fatal("duplicate or disordered type coverage admitted")
		}
	}
}
