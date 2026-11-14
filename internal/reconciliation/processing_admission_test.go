package reconciliation

import (
	"context"
	"errors"
	"oci-maintenance-router/internal/configuration"
	"oci-maintenance-router/internal/ocisecrets"
	"strings"
	"testing"
	"time"
)

func TestMaintenanceAdmissionRejectsMissingAuthorityBeforeWork(t *testing.T) {
	f := processingFixtureFor(t)
	for _, p := range []*MaintenanceProcessor{nil, {}} {
		r, err := p.Run(leaseBudget(t), f.live.lease)
		if err == nil || r.Snapshot().Completed {
			t.Fatal("zero processor ran")
		}
	}
	for _, ctx := range []context.Context{nil, context.Background()} {
		r, err := f.p.Run(ctx, f.live.lease)
		if err == nil || r.Snapshot().Completed {
			t.Fatal("unbounded request ran")
		}
	}
	ctx, cancel := context.WithCancel(leaseBudget(t))
	cancel()
	if _, err := f.p.Run(ctx, f.live.lease); !errors.Is(err, context.Canceled) {
		t.Fatal("cancellation identity lost", err)
	}
	expired, end := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer end()
	if _, err := f.p.Run(expired, f.live.lease); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("deadline identity lost", err)
	}
	if _, err := f.p.Run(leaseBudget(t), nil); err == nil {
		t.Fatal("missing lease ran")
	}
	other := acquireLease(t, leaseManager(t, &leaseMemory{}, leaseTime()), leaseBudget(t))
	if _, err := f.p.Run(leaseBudget(t), other); err == nil {
		t.Fatal("foreign lease ran")
	}
	for _, token := range []string{"", f.live.lease.record.OwnerToken} {
		f.p.token = func() string { return token }
		if _, err := f.p.Run(leaseBudget(t), f.live.lease); err == nil {
			t.Fatal("invalid attempt owner ran")
		}
	}
	if f.listed.Load() != 0 || f.sent.Load() != 0 {
		t.Fatal("invalid admission performed business work")
	}
}

func TestMaintenanceConstructorRejectsInvalidPinnedInputs(t *testing.T) {
	f := liveSelectorFixtureFor(t)
	for _, cfg := range [][]byte{nil, []byte("private-input"), []byte(candidateConfig)} {
		p, err := NewMaintenanceProcessor(cfg, []byte(preparationCatalog), candidateLocation(), strings.Repeat("b", 64), processingEmptySource(), f.objects, operatorClient(t), diagnosticVault(t), processingMySQL(t))
		if err == nil || p != nil || strings.Contains(err.Error(), "private-input") {
			t.Fatal("invalid config accepted/leaked")
		}
	}
	p, err := NewMaintenanceProcessor(f.config, nil, candidateLocation(), strings.Repeat("b", 64), processingEmptySource(), f.objects, operatorClient(t), diagnosticVault(t), processingMySQL(t))
	if err == nil || p != nil {
		t.Fatal("invalid catalog accepted")
	}
	p, err = NewMaintenanceProcessor(f.config, []byte(preparationCatalog), configuration.LedgerLocation{}, strings.Repeat("b", 64), processingEmptySource(), f.objects, operatorClient(t), diagnosticVault(t), processingMySQL(t))
	if err == nil || p != nil {
		t.Fatal("invalid location accepted")
	}
	p, err = NewMaintenanceProcessor(f.config, []byte(preparationCatalog), candidateLocation(), strings.Repeat("b", 64), nil, f.objects, operatorClient(t), diagnosticVault(t), processingMySQL(t))
	if err == nil || p != nil {
		t.Fatal("missing source accepted")
	}
	p, err = NewMaintenanceProcessor(f.config, []byte(preparationCatalog), candidateLocation(), strings.Repeat("b", 64), processingEmptySource(), f.objects, operatorClient(t), nil, processingMySQL(t))
	if err == nil || p != nil {
		t.Fatal("missing vault accepted")
	}
	p, err = NewMaintenanceProcessor(f.config, []byte(preparationCatalog), candidateLocation(), strings.Repeat("b", 64), processingEmptySource(), nil, operatorClient(t), diagnosticVault(t), processingMySQL(t))
	if err == nil || p != nil {
		t.Fatal("missing storage accepted")
	}
}

func TestMaintenanceConcreteCredentialBindingsRetainAdapterAdmission(t *testing.T) {
	f := liveSelectorFixtureFor(t)
	p, err := NewMaintenanceProcessor(f.config, []byte(preparationCatalog), candidateLocation(), strings.Repeat("b", 64), processingEmptySource(), f.objects, operatorClient(t), diagnosticVault(t), processingMySQL(t))
	if err != nil {
		t.Fatal(err)
	}
	_, err = p.current(context.Background(), "ocid1.vaultsecret.oc1.iad.payments")
	var e *ocisecrets.Error
	if !errors.As(err, &e) || e.Code != ocisecrets.DeadlineRequired {
		t.Fatal("CURRENT binding bypassed deadline admission", err)
	}
	_, err = p.version(leaseBudget(t), "ocid1.vaultsecret.oc1.iad.payments", 0)
	if !errors.As(err, &e) || e.Code != ocisecrets.InvalidRequest {
		t.Fatal("version binding bypassed exact-version admission", err)
	}
	for i := 0; i < 100; i++ {
		delay := p.jitter(time.Second)
		if delay < 0 || delay > time.Second {
			t.Fatal("default jitter out of range")
		}
	}
}
