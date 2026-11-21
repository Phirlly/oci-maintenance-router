package reconciliation

import (
	"strings"
	"testing"

	"github.com/oracle/oci-go-sdk/v65/common"
	"oci-maintenance-router/internal/ociownership"
)

func TestOwnershipConstructorsRejectWrongFixedBindings(t *testing.T) {
	f := liveSelectorFixtureFor(t)
	f.configure(t, func(v map[string]any) { v["schemaVersion"] = 7 })
	if r, err := NewRecoveryReviewer(f.config, []byte(preparationCatalog), candidateLocation(), processingEmptySource(), savedDetails{}, processingMySQL(t)); r == nil || err != nil {
		t.Fatal("valid reviewer control rejected", err)
	}
	provider := common.NewRawConfigurationProvider("synthetic", "synthetic", "us-ashburn-1", "synthetic", "synthetic", nil)
	for _, tc := range []struct{ kind, region, key, tenancy string }{
		{"instance", "us-ashburn-1", "iad", "ocid1.tenancy.oc1..example"},
		{"mysqldbsystem", "us-phoenix-1", "phx", "ocid1.tenancy.oc1..example"},
		{"mysqldbsystem", "us-ashburn-1", "phx", "ocid1.tenancy.oc1..example"},
		{"mysqldbsystem", "us-ashburn-1", "iad", "ocid1.tenancy.oc1..other"},
	} {
		makeClient := ociownership.NewMySQL
		if tc.kind == "instance" {
			makeClient = ociownership.NewCompute
		}
		client, err := makeClient(tc.region, tc.key, tc.tenancy, provider)
		if err != nil {
			t.Fatal(err)
		}
		clients := OwnershipClients{MySQL: map[string]*ociownership.Client{"us-ashburn-1": client}}
		p, err := NewMaintenanceProcessor(f.config, []byte(preparationCatalog), candidateLocation(), strings.Repeat("b", 64), processingEmptySource(), f.objects, operatorClient(t), diagnosticVault(t), clients)
		if p != nil || err == nil {
			t.Fatal("processor accepted wrong binding", tc)
		}
		r, err := NewRecoveryReviewer(f.config, []byte(preparationCatalog), candidateLocation(), processingEmptySource(), savedDetails{}, clients)
		if r != nil || err == nil {
			t.Fatal("reviewer accepted wrong binding", tc)
		}
	}
}

func TestSelectedComputeRequiresItsOwnFixedBindings(t *testing.T) {
	f := liveSelectorFixtureFor(t)
	cfg := resourceConfig(t, setupRuntimeConfig(t, "LIVE", "NONE", 0), "instance")
	for _, tc := range []struct {
		name    string
		clients OwnershipClients
		valid   bool
	}{
		{"Compute only", processingCompute(t), true},
		{"missing Compute", processingMySQL(t), false},
		{"wrong resource API", OwnershipClients{Compute: processingMySQL(t).MySQL}, false},
	} {
		p, err := NewMaintenanceProcessor(cfg, []byte(resourceCatalogWire), candidateLocation(), strings.Repeat("b", 64), processingEmptySource(), f.objects, operatorClient(t), diagnosticVault(t), tc.clients)
		if (err == nil && p != nil) != tc.valid {
			t.Fatal("processor binding", tc.name, err)
		}
		r, err := NewRecoveryReviewer(cfg, []byte(resourceCatalogWire), candidateLocation(), processingEmptySource(), savedDetails{}, tc.clients)
		if (err == nil && r != nil) != tc.valid {
			t.Fatal("reviewer binding", tc.name, err)
		}
	}
}
