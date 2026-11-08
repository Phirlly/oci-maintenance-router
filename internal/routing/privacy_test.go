package routing_test

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"oci-maintenance-router/internal/notices"
	"oci-maintenance-router/internal/routing"
)

func TestPlanContainsOnlySafeReferences(t *testing.T) {
	r := record(db, "private-malformed-id\n")
	r.Summary, r.Description, r.Reference = text("private-summary"), text("private-description"), text("private-reference")
	r.ChainID, r.AdditionalInformation = text("private-chain"), text("private-info")
	r.Resources[0].ResourceName = text("private-resource-name")
	r.Resources[0].AdditionalProperties = []notices.Property{{Name: text("private-property"), Value: text("private-value")}}
	lookup := metadata(db, "private-unmapped-owner")
	n := notice(t, r)
	got := policy(t, "defined").Resolve(n, false, map[string]routing.Lookup{db: lookup})
	if len(got.Decisions) != 2 || got.Decisions[1].Index != 1 || got.Decisions[1].ResourceID != "" {
		t.Fatalf("unsafe resource not retained by index: %+v", got)
	}
	data, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "private-") || strings.Contains(string(data), "vaultsecret") || strings.Contains(string(data), "Operations") || strings.Contains(string(data), "DefinedTags") {
		t.Fatalf("source content leaked into plan: %s", data)
	}
	if got.Revision != n.Key() {
		t.Fatal("safe notice identity lost")
	}
}

func TestReturnedCollectionsHaveIndependentOwnership(t *testing.T) {
	p, n := policy(t, "defined"), notice(t, record(db))
	lookups := map[string]routing.Lookup{db: metadata(db, "payments")}
	want := p.Resolve(n, false, lookups)
	got := p.Resolve(n, false, lookups)
	_, targets := p.Targets(n, false)
	if len(got.Groups) != 1 || len(got.Decisions) != 1 || len(targets) != 1 {
		t.Fatal("missing fixture work")
	}
	got.Groups[0].Resources[0].ResourceID = "changed-group"
	if got.Decisions[0].ResourceID != db {
		t.Fatal("group aliases decision")
	}
	got.Decisions[0].DestinationID = "changed-decision"
	targets[0].Region = "changed-target"
	if !reflect.DeepEqual(p.Resolve(n, false, lookups), want) {
		t.Fatal("output mutation changed policy or another plan")
	}
	if lookups[db].Metadata.DefinedTags["Operations"]["Owner"] != "payments" || n.Snapshot().Resources[0].ResourceID != db {
		t.Fatal("caller inputs mutated")
	}
}
