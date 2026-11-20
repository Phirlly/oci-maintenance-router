package reconciliation

import (
	"oci-maintenance-router/internal/configuration"
	"oci-maintenance-router/internal/ocinotifications"
	"oci-maintenance-router/internal/ocisecrets"
	"strings"
	"testing"
)

func TestSetupConstructorsRejectUnboundInputs(t *testing.T) {
	for _, kind := range []string{"config", "catalog", "legacy", "location", "backend", "channel", "release", "vault", "mode"} {
		t.Run(kind, func(t *testing.T) {
			f := setupRuntimeFixtureFor(t)
			cfg, catalog, location, release := f.config, []byte(preparationCatalog), candidateLocation(), strings.Repeat("b", 64)
			var objects PreparationObjects = f.objects
			var channel *ocinotifications.Client = operatorClient(t)
			var vault *ocisecrets.Client = diagnosticVault(t)
			switch kind {
			case "config":
				cfg = []byte("{}")
			case "catalog":
				catalog = []byte("{}")
			case "legacy":
				cfg = liveSchemaConfig(t, "TEST")
			case "location":
				location = configuration.LedgerLocation{}
			case "backend":
				objects = (*preparationObjects)(nil)
			case "channel":
				channel = nil
			case "release":
				release = "invalid"
			case "vault":
				vault = nil
			case "mode":
				cfg = setupRuntimeConfig(t, "LIVE", "NONE", 0)
			}
			if c, err := NewSetupCoordinator(cfg, catalog, location, release, objects, channel, vault); err == nil || c != nil {
				t.Fatal("unbound setup accepted", kind)
			}
		})
	}
}

func TestSetupReadinessConstructorsRequireLiveIntent(t *testing.T) {
	f := setupConfirmedRuntime(t)
	for _, mode := range []string{"PREVIEW", "PAUSED", "TEST"} {
		action := "NONE"
		if mode == "TEST" {
			action = "SEND_TEST"
		}
		if v, err := newSetupReadiness(setupRuntimeConfig(t, mode, action, 1), []byte(preparationCatalog), candidateLocation(), f.objects, operatorClient(t)); err == nil || v != nil {
			t.Fatal("non-LIVE readiness constructed", mode)
		}
	}
	if v, err := newSetupReadiness(setupRuntimeConfig(t, "LIVE", "NONE", 0), []byte(preparationCatalog), candidateLocation(), f.objects, nil); err == nil || v != nil {
		t.Fatal("missing channel accepted")
	}
	if v, err := newSetupReadiness([]byte("{}"), []byte(preparationCatalog), candidateLocation(), f.objects, operatorClient(t)); err == nil || v != nil {
		t.Fatal("invalid configuration accepted")
	}
}
