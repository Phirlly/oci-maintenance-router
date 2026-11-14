package reconciliation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"

	"oci-maintenance-router/internal/configuration"
	"oci-maintenance-router/internal/ocisecrets"
)

func TestDiagnosticCoordinatorConstructorOwnsValidatedTESTIntent(t *testing.T) {
	f := coordinatorFixture(t)
	cfg, cat := diagnosticConfig(t), []byte(preparationCatalog)
	d, err := NewDiagnosticCoordinator(cfg, cat, candidateLocation(), strings.Repeat("b", 64), f.objects, diagnosticVault(t))
	if err != nil {
		t.Fatal(err)
	}
	configHash, catalogHash := sha256.Sum256(cfg), sha256.Sum256(cat)
	clear(cfg)
	clear(cat)
	if d.controls.configHash != configHash || d.controls.catalogHash != catalogHash || d.intent.CatalogSHA256 != hex.EncodeToString(catalogHash[:]) || d.intent.RequestID != "test-1" || d.intent.EnvironmentID != f.lease.manager.environmentID || !d.intent.NonProduction {
		t.Fatal("constructor did not bind owned trusted inputs")
	}
	// Production closure reaches the real adapter; invalid input is rejected
	// before signing/network. Success paths remain adapter-owned loopback tests.
	_, err = d.current(context.Background(), "")
	if e, ok := err.(*ocisecrets.Error); !ok || e.Code != ocisecrets.InvalidRequest {
		t.Fatal("not bound to real Vault adapter", err)
	}
}

func TestDiagnosticCoordinatorRejectsInvalidConstructorInputs(t *testing.T) {
	f := coordinatorFixture(t)
	for _, kind := range []string{"schema", "config", "catalog", "location", "release", "objects", "typed-nil", "vault", "PREVIEW", "PAUSED"} {
		t.Run(kind, func(t *testing.T) {
			cfg, cat := diagnosticConfig(t), []byte(preparationCatalog)
			location := candidateLocation()
			release := strings.Repeat("b", 64)
			var objects DiagnosticObjects = f.objects
			vault := diagnosticVault(t)
			want := "invalid_request"
			switch kind {
			case "schema":
				cfg = preparationConfig(t)
			case "config":
				cfg = []byte("private")
			case "catalog":
				cat = []byte("private")
			case "location":
				location = configuration.LedgerLocation{}
			case "release":
				release = "private"
			case "objects":
				objects = nil
			case "typed-nil":
				var s *preparationObjects
				objects = s
			case "vault":
				vault = nil
			case "PREVIEW", "PAUSED":
				cfg = diagnosticConfig(t, func(c map[string]any) { c["mode"] = kind })
				want = "intent_required"
			}
			d, err := NewDiagnosticCoordinator(cfg, cat, location, release, objects, vault)
			diagnosticRunFailure(t, err, want)
			if d != nil {
				t.Fatal("invalid constructor returned coordinator")
			}
		})
	}
}

func TestDiagnosticCoordinatorRejectsInvalidRunBeforeIO(t *testing.T) {
	for _, kind := range []string{"nil-owner", "zero-owner", "nil-lease", "foreign-lease", "nil-context", "no-deadline", "canceled", "token", "owner-token"} {
		t.Run(kind, func(t *testing.T) {
			f := coordinatorFixture(t)
			d, l, ctx := f.coordinator, f.lease, leaseBudget(t)
			want := "invalid_request"
			switch kind {
			case "nil-owner":
				d = nil
			case "zero-owner":
				d = &DiagnosticCoordinator{}
			case "nil-lease":
				l = nil
			case "foreign-lease":
				l.manager.environmentID = strings.Repeat("a", 64)
				want = "environment_mismatch"
			case "nil-context":
				ctx = nil
			case "no-deadline":
				ctx = context.Background()
				want = "deadline_required"
			case "canceled":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
				want = "canceled"
			case "token":
				d.token = func() string { return "invalid" }
			case "owner-token":
				d.token = func() string { return l.record.OwnerToken }
			}
			before := len(f.objects.calls)
			got, err := d.Run(ctx, l)
			diagnosticRunFailure(t, err, want)
			if got.Dispatched() || got.Record().Snapshot().State != "" || len(f.objects.calls) != before || f.reads.Load() != 0 || f.sends.Load() != 0 {
				t.Fatal("invalid run had effects")
			}
		})
	}
}
