package reconciliation

import (
	"bytes"
	"reflect"
	"testing"

	"oci-maintenance-router/internal/notices"
)

func TestPreparationPersistsInitialAndReturnsCoveredHistoryWithoutScanning(t *testing.T) {
	n := candidateNotice(t, 1, nil, true)
	p, s, l := preparationFixture(t, []notices.Notice{n})
	if err := p.Initialize(leaseBudget(t), l); err != nil {
		t.Fatal(err)
	}
	b := prepareSuccessfully(t, p, l)
	body, _ := b.Encode()
	h := preparationHead(t, s, l)
	if !b.Suppresses("us-ashburn-1", baselineCompartment, n.Key()) || h.BaselineID() != b.ID() || h.BaselineDigest() != preparedReference(b).Digest {
		t.Fatal("confirmed baseline/head do not preserve discovery")
	}
	before := preparationCalls(s, "LIST", "")
	got := prepareSuccessfully(t, p, l)
	actual, _ := got.Encode()
	if !bytes.Equal(body, actual) || preparationCalls(s, "LIST", "") != before {
		t.Fatal("covered history was rebuilt")
	}
}

func TestPreparationScopeExpansionDoesNotRebaselineOldCells(t *testing.T) {
	old, arrived := candidateNotice(t, 1, nil, true), candidateNotice(t, 2, nil, true)
	p, s, l := preparationFixture(t, []notices.Notice{old})
	if err := p.Initialize(leaseBudget(t), l); err != nil {
		t.Fatal(err)
	}
	first := prepareSuccessfully(t, p, l)
	expandedConfig := preparationConfig(t, func(c map[string]any) { c["compartmentIds"] = []string{baselineOtherCompartment, baselineCompartment} })
	p = preparationFor(t, s, expandedConfig, []notices.Notice{old, arrived})
	next := prepareSuccessfully(t, p, l)
	if len(next.Layers()) != 2 || !reflect.DeepEqual(first.Layers()[0], next.Layers()[0]) || next.Suppresses("us-ashburn-1", baselineCompartment, arrived.Key()) || !next.Suppresses("us-ashburn-1", baselineOtherCompartment, arrived.Key()) {
		t.Fatal("expansion reset old history or omitted new cells")
	}
	body, _ := next.Encode()
	for _, cfg := range [][]byte{preparationConfig(t), expandedConfig, preparationConfig(t, func(c map[string]any) {
		c["mysqlServices"] = []string{"MySQL"}
		c["ownership"] = map[string]any{"kind": "freeform", "key": "team"}
	})} {
		p = preparationFor(t, s, cfg, nil)
		before := preparationCalls(s, "LIST", "")
		got := prepareSuccessfully(t, p, l)
		actual, _ := got.Encode()
		if !bytes.Equal(body, actual) || preparationCalls(s, "LIST", "") != before {
			t.Fatal("removal, readdition or policy edit rebaselined covered scope")
		}
	}
}
