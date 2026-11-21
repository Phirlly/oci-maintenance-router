package reconciliation

import (
	"bytes"
	"reflect"
	"testing"
)

func TestPreparationRequiresAndRetainsResourceQualifiedCoverage(t *testing.T) {
	p, objects, lease := preparationFixture(t, nil)
	if err := p.Initialize(leaseBudget(t), lease); err != nil {
		t.Fatal(err)
	}
	first := prepareSuccessfully(t, p, lease)
	// Exercise the real guards with a private synthetic type selection; this
	// does not claim configuration or real Compute discovery is implemented.
	p.discovery.scope.resourceTypes = []string{"instance", "mysqldbsystem"}
	expanded := prepareSuccessfully(t, p, lease)
	if expanded.ID() == first.ID() || len(expanded.Layers()) != 2 || !reflect.DeepEqual(first.Layers()[0], expanded.Layers()[0]) {
		t.Fatal("preparation reused another resource type's coverage")
	}
	before := baselineWire(t, expanded)
	for _, types := range [][]string{{"instance", "mysqldbsystem"}, {"mysqldbsystem"}, {"instance", "mysqldbsystem"}} {
		p.discovery.scope.resourceTypes = types
		reads := preparationCalls(objects, "LIST", "")
		got := prepareSuccessfully(t, p, lease)
		if !bytes.Equal(before, baselineWire(t, got)) || preparationCalls(objects, "LIST", "") != reads {
			t.Fatal("covered resource scope rescanned or reset history")
		}
	}
}

func TestLiveSelectorRefusesUnpreparedResourceType(t *testing.T) {
	f := liveSelectorFixtureFor(t)
	f.selector.scope.resourceTypes = []string{"instance", "mysqldbsystem"}
	b, err := f.selector.Select(leaseBudget(t), f.lease)
	liveSelectionFailure(t, b, err, "scope_unprepared")
	if preparationCalls(f.objects, "CREATE", baselineSelectionName) != 0 {
		t.Fatal("unprepared resource type was selected")
	}
	p := preparationFor(t, f.objects, preparationConfig(t), nil)
	p.discovery.scope.resourceTypes = []string{"instance", "mysqldbsystem"}
	expanded := prepareSuccessfully(t, p, f.lease)
	f.configure(t)
	f.selector.scope.resourceTypes = []string{"instance", "mysqldbsystem"}
	if got := f.selectOK(t); !bytes.Equal(baselineWire(t, got), baselineWire(t, expanded)) {
		t.Fatal("selection did not retain prepared resource coverage")
	}
}
