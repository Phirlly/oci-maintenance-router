package reconciliation

import (
	"os"
	"testing"

	"oci-maintenance-router/internal/configuration"
	"oci-maintenance-router/internal/delivery"
	"oci-maintenance-router/internal/noticeledger"
	"oci-maintenance-router/internal/ocistorage"
	"oci-maintenance-router/internal/routing"
)

func selectionInputs(t *testing.T) ([]byte, []byte, configuration.Catalog) {
	t.Helper()
	read := func(name string) []byte {
		b, err := os.ReadFile("../configuration/testdata/" + name + ".json")
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	data, wire := read("selected"), read("selected-catalog")
	cat, err := configuration.DecodeCatalog(wire)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = configuration.Decode(data, cat); err != nil {
		t.Fatal("fixture must reach admission", err)
	}
	return data, wire, cat
}

func TestResourceSelectionPolicyConsumersAdmitWithoutOperations(t *testing.T) {
	data, wire, cat := selectionInputs(t)
	location := candidateLocation()
	accepted := func(name string, empty bool, err error) {
		t.Helper()
		if empty || err != nil {
			t.Fatalf("%s rejected selected resources: %v", name, err)
		}
	}
	p, err := routing.New(data, cat)
	accepted("policy", p == nil, err)
	b, err := delivery.New(data, cat, location)
	accepted("builder", b == nil, err)
	o, err := noticeledger.New(data, cat, location)
	accepted("observer", o == nil, err)
	c, err := NewCandidates(data, cat, location)
	accepted("candidates", c == nil, err)
	a, s := detailSources(t, nil, nil)
	collector, err := NewCollector(data, cat, location, a, s)
	accepted("collector", collector == nil, err)
	objects := &discoveryObjects{objects: map[string]ocistorage.Object{}}
	d, err := NewDiscovery(data, wire, location, a, objects)
	if d == nil || err != nil || len(objects.calls) != 0 {
		t.Fatal("discovery rejected selection or performed operations", err)
	}
}
