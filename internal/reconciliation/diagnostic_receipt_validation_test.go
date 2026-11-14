package reconciliation

import (
	"context"
	"crypto/sha256"
	"strings"
	"testing"

	"oci-maintenance-router/internal/configuration"
)

func TestDiagnosticReceiptConstructorOwnsInputsAndRejectsInvalidIntent(t *testing.T) {
	f := coordinatorFixture(t)
	r := f.seed(t, DiagnosticAccepted, false)
	cfg, cat := receiptConfig(t, r), []byte(preparationCatalog)
	v, err := NewDiagnosticReceiptVerifier(cfg, cat, candidateLocation(), strings.Repeat("b", 64), f.objects)
	if err != nil {
		t.Fatal(err)
	}
	f.objects.seed("configuration/router.json", cfg)
	hash := sha256.Sum256(cfg)
	clear(cfg)
	clear(cat)
	got, err := v.Verify(leaseBudget(t), f.lease)
	if err != nil || got.configHash != hash || !got.Record().Matches(r.Snapshot().Intent) {
		t.Fatal("caller mutation changed verifier", err)
	}
	for _, kind := range []string{"schema1", "schema2", "schema3", "config", "catalog", "location", "release", "objects", "typed-nil", "no-diagnostic", "no-code", "empty-code"} {
		t.Run(kind, func(t *testing.T) {
			cfg, cat := receiptConfig(t, r), []byte(preparationCatalog)
			location, release := candidateLocation(), strings.Repeat("b", 64)
			var objects DiagnosticObjects = f.objects
			want := "invalid_request"
			switch kind {
			case "schema1":
				cfg = []byte(candidateConfig)
			case "schema2":
				cfg = preparationConfig(t)
			case "schema3":
				cfg = diagnosticConfig(t)
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
				objects = (*preparationObjects)(nil)
			case "no-diagnostic":
				cfg = receiptConfig(t, r, func(c map[string]any) { c["mode"] = "PREVIEW"; delete(c, "diagnostic") })
				want = "intent_required"
			case "no-code", "empty-code":
				cfg = receiptConfig(t, r, func(c map[string]any) {
					d := c["diagnostic"].(map[string]any)
					if kind == "no-code" {
						delete(d, "receiptCode")
					} else {
						d["receiptCode"] = ""
					}
				})
				want = "receipt_required"
			}
			before := len(f.objects.calls)
			got, err := NewDiagnosticReceiptVerifier(cfg, cat, location, release, objects)
			diagnosticRunFailure(t, err, want)
			if got != nil || len(f.objects.calls) != before || strings.Contains(err.Error(), "private") {
				t.Fatal("invalid constructor performed I/O, leaked input or returned verifier")
			}
		})
	}
}

func TestDiagnosticReceiptRejectsInvalidCallsBeforeIO(t *testing.T) {
	for _, kind := range []string{"nil-verifier", "zero-verifier", "nil-lease", "foreign-lease", "nil-context", "no-deadline", "canceled"} {
		f := coordinatorFixture(t)
		v := receiptVerifier(t, f, receiptConfig(t, f.seed(t, DiagnosticAccepted, false)))
		lease, ctx := f.lease, leaseBudget(t)
		want := "invalid_request"
		switch kind {
		case "nil-verifier":
			v = nil
		case "zero-verifier":
			v = &DiagnosticReceiptVerifier{}
		case "nil-lease":
			lease = nil
		case "foreign-lease":
			lease.manager.environmentID = strings.Repeat("a", 64)
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
		}
		before := len(f.objects.calls)
		got, err := v.Verify(ctx, lease)
		receiptFailure(t, got, err, want)
		if len(f.objects.calls) != before {
			t.Fatal("invalid verification reached backend", kind)
		}
	}
}
