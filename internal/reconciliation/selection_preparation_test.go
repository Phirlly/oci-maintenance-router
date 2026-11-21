package reconciliation

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"oci-maintenance-router/internal/notices"
)

func resourcePreparation(t *testing.T, s *preparationObjects, ns []notices.Notice, kinds ...string) *BaselinePreparation {
	t.Helper()
	legacy := preparationFor(t, s, preparationConfig(t), ns)
	cfg := resourceConfig(t, preparationConfig(t), kinds...)
	s.seed("configuration/router.json", cfg)
	s.seed("configuration/catalog.json", []byte(resourceCatalogWire))
	p, err := NewBaselinePreparation(cfg, []byte(resourceCatalogWire), candidateLocation(), legacy.discovery.collector.announcements, s)
	if err != nil {
		t.Fatal("selected preparation rejected", err)
	}
	p.discovery.now, p.discovery.wait = legacy.discovery.now, legacy.discovery.wait
	return p
}

func TestSelectedPreparationUsesTypedCumulativeHistory(t *testing.T) {
	old, arrived := candidateNotice(t, 1, nil, true), candidateNotice(t, 2, nil, true)
	p, s, l := preparationFixture(t, []notices.Notice{old})
	if err := p.Initialize(leaseBudget(t), l); err != nil {
		t.Fatal(err)
	}
	first := prepareSuccessfully(t, p, l)
	p = resourcePreparation(t, s, []notices.Notice{old, arrived}, "mysqldbsystem")
	before := preparationCalls(s, "LIST", "")
	got := prepareSuccessfully(t, p, l)
	a, _ := first.Encode()
	b, _ := got.Encode()
	if !bytes.Equal(a, b) || preparationCalls(s, "LIST", "") != before {
		t.Fatal("MySQL-only adoption reset covered history")
	}
	p = resourcePreparation(t, s, []notices.Notice{old, arrived}, "instance", "mysqldbsystem")
	next := prepareSuccessfully(t, p, l)
	if len(next.Layers()) != 2 || !reflect.DeepEqual(first.Layers()[0], next.Layers()[0]) || len(next.Layers()[1].Cells) != 1 || next.Layers()[1].Cells[0].ResourceType != "instance" {
		t.Fatal("expansion replaced old cells", next.Layers())
	}
	if next.Suppresses("mysqldbsystem", "us-ashburn-1", baselineCompartment, arrived.Key()) || !next.Suppresses("instance", "us-ashburn-1", baselineCompartment, arrived.Key()) {
		t.Fatal("type-qualified suppression lost")
	}
	want, _ := next.Encode()
	for _, kinds := range [][]string{{"instance"}, {"mysqldbsystem"}, {"mysqldbsystem", "instance"}} {
		p = resourcePreparation(t, s, nil, kinds...)
		before = preparationCalls(s, "LIST", "")
		got = prepareSuccessfully(t, p, l)
		b, _ = got.Encode()
		if !bytes.Equal(want, b) || preparationCalls(s, "LIST", "") != before {
			t.Fatal("selection edit reset history or rescanned")
		}
	}
}

func TestSelectedPreparationIntentBindsTypesAndPreservesOrderIndependence(t *testing.T) {
	_, s, _ := preparationFixture(t, nil)
	p := resourcePreparation(t, s, nil, "mysqldbsystem", "instance")
	h, err := NewPreparedHistory(strings.Repeat("a", 64), "installation-1")
	if err != nil {
		t.Fatal(err)
	}
	wire := `["baseline-intent-v2","aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",null,["us-ashburn-1"],["ocid1.compartment.oc1..example"],{"instance":["Oracle Cloud Infrastructure Compute"],"mysqldbsystem":["MySQL","Oracle HeatWave MySQL"]}]`
	want := fmt.Sprintf("p2-%x", sha256.Sum256([]byte(wire)))
	if p.targetID(h) != want {
		t.Fatal("selected intent differs from independent tuple", p.targetID(h), want)
	}
	other := resourcePreparation(t, s, nil, "instance", "mysqldbsystem")
	if other.targetID(h) != want {
		t.Fatal("selection order changed intent")
	}
	mysql := resourcePreparation(t, s, nil, "mysqldbsystem")
	compute := resourcePreparation(t, s, nil, "instance")
	if mysql.targetID(h) == compute.targetID(h) || mysql.targetID(h) == want || compute.targetID(h) == want {
		t.Fatal("different selected cells share interrupted target")
	}
	if len(compute.discovery.scope.resourceTypes) != 1 || compute.discovery.scope.resourceTypes[0] != "instance" {
		t.Fatal("Compute-only scope contains MySQL")
	}
}
