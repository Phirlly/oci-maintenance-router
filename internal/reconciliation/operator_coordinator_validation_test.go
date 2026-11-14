package reconciliation

import (
	"context"
	"crypto/sha256"
	"testing"

	"oci-maintenance-router/internal/configuration"
	"oci-maintenance-router/internal/ocinotifications"
	"oci-maintenance-router/internal/ocistorage"
)

func TestOperatorCoordinatorOwnsPinnedIntentWithoutIO(t *testing.T) {
	s := &preparationObjects{discoveryObjects: &discoveryObjects{objects: map[string]ocistorage.Object{}}}
	cfg, cat := diagnosticConfig(t, operatorTestConfig), []byte(preparationCatalog)
	d, err := NewOperatorCoordinator(cfg, cat, candidateLocation(), s, operatorClient(t))
	if err != nil || d == nil {
		t.Fatal("valid operator intent rejected", err)
	}
	want := OperatorChannel{"ocid1.onstopic.oc1.iad.operator", "ocid1.onssubscription.oc1.iad.operator", "EMAIL", "Team@example.invalid"}
	if d.desired != want || !validOperatorIntent(d.intent) || d.intent.RequestID != "operator-1" || d.intent.InstallationID != "installation-1" || d.controls.configHash != sha256.Sum256(cfg) || d.controls.catalogHash != sha256.Sum256(cat) || d.read == nil || d.token == nil || len(s.calls) != 0 {
		t.Fatal("constructor lost pinned controls or performed I/O")
	}
	clear(cfg)
	clear(cat)
	if d.desired != want || !validOperatorIntent(d.intent) {
		t.Fatal("caller mutated pinned intent")
	}
	// Exercise the real bound reader without credentials or networking: the
	// adapter refuses a missing deadline before any signed request.
	_, err = d.read(context.Background())
	if e, ok := err.(*ocinotifications.Error); !ok || e.Code != ocinotifications.DeadlineRequired {
		t.Fatal("reader not bound to real adapter", err)
	}
}

func TestOperatorCoordinatorRejectsInvalidConstruction(t *testing.T) {
	for _, kind := range []string{"config", "catalog", "schema", "missing-intent", "preview", "paused", "location", "objects", "typed-nil", "client"} {
		t.Run(kind, func(t *testing.T) {
			cfg, cat := diagnosticConfig(t, operatorTestConfig), []byte(preparationCatalog)
			location := candidateLocation()
			var objects OperatorChallengeObjects = &preparationObjects{discoveryObjects: &discoveryObjects{}}
			var client *ocinotifications.Client = operatorClient(t)
			want := "invalid_request"
			switch kind {
			case "config":
				cfg = []byte("private config")
			case "catalog":
				cat = []byte("private catalog")
			case "schema":
				cfg = diagnosticConfig(t)
			case "missing-intent":
				cfg = diagnosticConfig(t, func(c map[string]any) { c["schemaVersion"] = 5 })
				want = "intent_required"
			case "preview", "paused":
				cfg = diagnosticConfig(t, operatorTestConfig, func(c map[string]any) {
					if kind == "preview" {
						c["mode"] = "PREVIEW"
					} else {
						c["mode"] = "PAUSED"
					}
				})
				want = "intent_required"
			case "location":
				location = configuration.LedgerLocation{}
			case "objects":
				objects = nil
			case "typed-nil":
				objects = (*preparationObjects)(nil)
			case "client":
				client = nil
			}
			d, err := NewOperatorCoordinator(cfg, cat, location, objects, client)
			operatorRunFailure(t, err, want)
			if d != nil {
				t.Fatal("invalid constructor returned coordinator")
			}
		})
	}
}
