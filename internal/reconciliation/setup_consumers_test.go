package reconciliation

import (
	"strings"
	"testing"
)

func TestSetupConsumersAcceptCodeFreeLiveConfiguration(t *testing.T) {
	f := setupConfirmedRuntime(t)
	cfg := setupRuntimeConfig(t, "LIVE", "NONE", 0)
	f.objects.seed("configuration/router.json", cfg)
	if _, err := NewLiveBaselineSelector(cfg, []byte(preparationCatalog), candidateLocation(), strings.Repeat("b", 64), f.objects, operatorClient(t)); err != nil {
		t.Fatal("LIVE selector rejected saved setup", err)
	}
	if _, err := NewIssueReporter(cfg, []byte(preparationCatalog), candidateLocation(), f.objects, operatorClient(t)); err != nil {
		t.Fatal("operator reporter rejected saved setup", err)
	}
	if _, err := NewMaintenanceProcessor(cfg, []byte(preparationCatalog), candidateLocation(), strings.Repeat("b", 64), announcementDetails{}, f.objects, operatorClient(t), diagnosticVault(t), processingMySQL(t)); err != nil {
		t.Fatal("processing rejected saved setup", err)
	}
}
