package reconciliation

import (
	"context"
	"errors"
	"oci-maintenance-router/internal/configuration"
	"strings"
	"testing"
	"time"
)

func TestSetupOperationsRejectZeroForeignOrExpiredAuthority(t *testing.T) {
	f := setupConfirmedRuntime(t)
	ctx := leaseBudget(t)
	var zero *SetupCoordinator
	if _, err := zero.Run(ctx, f.lease); err == nil {
		t.Fatal("nil coordinator accepted")
	}
	if _, err := new(SetupCoordinator).Run(ctx, f.lease); err == nil {
		t.Fatal("zero coordinator accepted")
	}
	if _, err := f.coordinator.Run(ctx, nil); err == nil {
		t.Fatal("missing lease accepted")
	}
	foreign := candidateLocation()
	foreign.Namespace = "another"
	c, err := NewSetupCoordinator(f.config, []byte(preparationCatalog), foreign, strings.Repeat("b", 64), f.objects, operatorClient(t), nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = c.Run(ctx, f.lease); err == nil {
		t.Fatal("foreign environment accepted")
	}
	cfg := setupRuntimeConfig(t, "LIVE", "NONE", 0)
	v, err := newSetupReadiness(cfg, []byte(preparationCatalog), foreign, f.objects, operatorClient(t))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = v.verifyOperator(ctx, f.lease); err == nil {
		t.Fatal("foreign readiness accepted")
	}
	for _, v := range []*setupReadiness{nil, {}} {
		if _, err = v.verifyOperator(ctx, f.lease); err == nil {
			t.Fatal("zero readiness accepted")
		}
	}
	v = setupReadinessFor(t, f)
	if err = v.verify(ctx, f.lease, "invalid release"); err == nil {
		t.Fatal("unbound release accepted")
	}
	expired, cancel := context.WithDeadline(ctx, time.Now().Add(-time.Second))
	defer cancel()
	if err = v.verify(expired, f.lease, strings.Repeat("b", 64)); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("expired readiness did not preserve timeout", err)
	}
	f.objects.seed("configuration/router.json", []byte("changed"))
	if err = v.verify(ctx, f.lease, strings.Repeat("b", 64)); err == nil {
		t.Fatal("old pinned configuration accepted")
	}
}

func TestSetupReadinessRequiresActualChannelLookup(t *testing.T) {
	f := setupConfirmedRuntime(t)
	cfg := setupRuntimeConfig(t, "LIVE", "NONE", 0)
	f.objects.seed("configuration/router.json", cfg)
	v, err := newSetupReadiness(cfg, []byte(preparationCatalog), candidateLocation(), f.objects, operatorClient(t))
	if err != nil {
		t.Fatal(err)
	}
	// The concrete client has synthetic invalid signing material and cannot
	// dispatch HTTP. Saved proof alone must not bypass its current channel read.
	got, err := v.verifyOperator(leaseBudget(t), f.lease)
	var detail *OperatorRunError
	if !errors.As(err, &detail) || detail.Code != "channel_read" || got.Record().Snapshot().State != "" {
		t.Fatal("saved state bypassed current channel", err)
	}
}

func TestSetupReporterRejectsInvalidOrUnboundConfiguration(t *testing.T) {
	f := setupConfirmedRuntime(t)
	for _, kind := range []string{"catalog", "location", "legacy-proof", "legacy-mode"} {
		cfg, catalog, location := setupRuntimeConfig(t, "LIVE", "NONE", 0), []byte(preparationCatalog), candidateLocation()
		switch kind {
		case "catalog":
			catalog = []byte("{}")
		case "location":
			location = configuration.LedgerLocation{}
		case "legacy-proof":
			cfg = liveSchemaConfig(t, "LIVE", func(v map[string]any) { v["schemaVersion"] = 5 })
		case "legacy-mode":
			cfg = preparationConfig(t)
		}
		if p, err := NewIssueReporter(cfg, catalog, location, f.objects, operatorClient(t)); err == nil || p != nil {
			t.Fatal("invalid reporting authority accepted", kind)
		}
	}
}
