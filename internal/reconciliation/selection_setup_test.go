package reconciliation

import (
	"strings"
	"testing"
)

func TestSelectedSetupLifecycleAndSelectionReuse(t *testing.T) {
	f := setupRuntimeFixtureFor(t)
	f.resourceTypes = []string{"mysqldbsystem"}
	f.configure(t, "SEND_TEST", 1)
	var err error
	f.result, err = f.coordinator.Run(leaseBudget(t), f.lease)
	if err != nil {
		t.Fatal("selected setup failed", err)
	}
	if _, err = f.coordinator.Run(leaseBudget(t), f.lease); err != nil {
		t.Fatal("same test retry failed", err)
	}
	f.configure(t, "CONFIRM", 1)
	if _, err = f.coordinator.Run(leaseBudget(t), f.lease); err != nil {
		t.Fatal("selected confirmation failed", err)
	}
	for _, kinds := range [][]string{{"mysqldbsystem"}, {"instance", "mysqldbsystem"}, {"instance"}} {
		cfg := resourceConfig(t, setupRuntimeConfig(t, "LIVE", "NONE", 0), kinds...)
		f.objects.seed("configuration/router.json", cfg)
		s, err := NewLiveBaselineSelector(cfg, []byte(resourceCatalogWire), candidateLocation(), strings.Repeat("b", 64), f.objects, operatorClient(t))
		if err != nil {
			t.Fatal(err)
		}
		s.setup.read = f.coordinator.read
		if len(kinds) == 2 {
			if _, err = s.Select(leaseBudget(t), f.lease); err == nil {
				t.Fatal("unprepared Compute activated")
			}
			p := resourcePreparation(t, f.objects, nil, kinds...)
			prepareSuccessfully(t, p, f.lease)
			f.objects.seed("configuration/router.json", cfg)
		}
		if _, err = s.Select(leaseBudget(t), f.lease); err != nil {
			t.Fatal("selection edit invalidated unchanged setup", err)
		}
	}
	if f.diagnosticSends != 1 || f.operatorSends != 1 {
		t.Fatal("confirmation/activation repeated tests")
	}
}

func TestSelectedCatalogRequiresNewTestAttemptAndConfirmation(t *testing.T) {
	f := setupConfirmedRuntime(t)
	f.resourceTypes = []string{"mysqldbsystem"}
	cfg := resourceConfig(t, setupRuntimeConfig(t, "LIVE", "NONE", 0), "mysqldbsystem")
	f.objects.seed("configuration/router.json", cfg)
	f.objects.seed("configuration/catalog.json", []byte(resourceCatalogWire))
	s, err := NewLiveBaselineSelector(cfg, []byte(resourceCatalogWire), candidateLocation(), strings.Repeat("b", 64), f.objects, operatorClient(t))
	if err != nil {
		t.Fatal(err)
	}
	s.setup.read = f.coordinator.read
	if _, err = s.Select(leaseBudget(t), f.lease); err == nil {
		t.Fatal("old catalog proof activated new catalog")
	}
	f.configure(t, "SEND_TEST", 1)
	if _, err = f.coordinator.Run(leaseBudget(t), f.lease); err == nil {
		t.Fatal("same attempt silently changed intent")
	}
	if f.diagnosticSends != 1 || f.operatorSends != 1 {
		t.Fatal("conflict sent tests")
	}
	f.configure(t, "SEND_TEST", 2)
	f.result, err = f.coordinator.Run(leaseBudget(t), f.lease)
	if err != nil || f.diagnosticSends != 2 {
		t.Fatal("explicit new diagnostic failed", err)
	}
	f.configure(t, "CONFIRM", 2)
	if _, err = f.coordinator.Run(leaseBudget(t), f.lease); err != nil {
		t.Fatal(err)
	}
	f.objects.seed("configuration/router.json", cfg)
	if _, err = s.Select(leaseBudget(t), f.lease); err != nil {
		t.Fatal("new confirmed catalog did not activate", err)
	}
}
