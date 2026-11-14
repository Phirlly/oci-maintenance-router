package reconciliation

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"

	"oci-maintenance-router/internal/ocinotifications"
	"oci-maintenance-router/internal/pagerduty"
)

type liveSelectorFixture struct {
	selector                       *LiveBaselineSelector
	objects                        *preparationObjects
	lease                          *Lease
	baseline                       Baseline
	diagnostic                     DiagnosticRecord
	operator                       OperatorChallenge
	config                         []byte
	channelReads                   atomic.Int32
	diagnosticSends, operatorSends atomic.Int32
}

func liveSelectionFailure(t *testing.T, b Baseline, err error, code string) *LiveSelectionError {
	t.Helper()
	var e *LiveSelectionError
	if !errors.As(err, &e) || e.Code != code {
		t.Fatalf("error=%v, want %s", err, code)
	}
	if b.ID() != "" || b.EnvironmentID() != "" || b.Layers() != nil {
		t.Fatal("failure returned baseline")
	}
	return e
}

func liveSelectorFixtureFor(t *testing.T) *liveSelectorFixture {
	t.Helper()
	p, objects, lease := preparationFixture(t, nil)
	if err := p.Initialize(leaseBudget(t), lease); err != nil {
		t.Fatal(err)
	}
	f := &liveSelectorFixture{objects: objects, lease: lease, baseline: prepareSuccessfully(t, p, lease)}
	cfg := liveSchemaConfig(t, "TEST")
	objects.seed("configuration/router.json", cfg)
	d, err := NewDiagnosticCoordinator(cfg, []byte(preparationCatalog), candidateLocation(), strings.Repeat("b", 64), objects, diagnosticVault(t))
	if err != nil {
		t.Fatal(err)
	}
	d.current = func(context.Context, string) (diagnosticCredential, error) {
		return diagnosticMaterial{7, func() []byte { return []byte("synthetic-key") }}, nil
	}
	d.send = func(context.Context, []byte, []byte) (pagerduty.Response, error) {
		f.diagnosticSends.Add(1)
		return pagerduty.Response{Status: 202}, nil
	}
	dr, err := d.Run(leaseBudget(t), lease)
	if err != nil || dr.Record().Snapshot().State != DiagnosticAccepted {
		t.Fatal("diagnostic fixture", err)
	}
	f.diagnostic = dr.Record()
	o, err := NewOperatorCoordinator(cfg, []byte(preparationCatalog), candidateLocation(), objects, operatorClient(t))
	if err != nil {
		t.Fatal(err)
	}
	o.read = func(context.Context) (operatorObservation, error) {
		return operatorObservation{o.desired, func(context.Context, []byte) (ocinotifications.Publication, error) {
			f.operatorSends.Add(1)
			return ocinotifications.Publication{Status: 200, MessageID: "synthetic-message", MayHaveSent: true}, nil
		}}, nil
	}
	or, err := o.Run(leaseBudget(t), lease)
	if err != nil || or.Record().Snapshot().State != OperatorAccepted {
		t.Fatal("operator fixture", err)
	}
	f.operator = or.Record()
	f.configure(t)
	return f
}

func (f *liveSelectorFixture) configure(t *testing.T, changes ...func(map[string]any)) {
	t.Helper()
	cfg := liveSchemaConfig(t, "LIVE", append([]func(map[string]any){func(v map[string]any) {
		v["diagnostic"].(map[string]any)["receiptCode"] = f.diagnostic.Snapshot().ConfirmationCode
		v["operatorTest"].(map[string]any)["receiptCode"] = f.operator.Snapshot().ConfirmationCode
	}}, changes...)...)
	s, err := NewLiveBaselineSelector(cfg, []byte(preparationCatalog), candidateLocation(), strings.Repeat("b", 64), f.objects, operatorClient(t))
	if err != nil {
		t.Fatal("valid LIVE selector", err)
	}
	s.operator.read = func(context.Context) (OperatorChannel, error) {
		f.channelReads.Add(1)
		return s.operator.input.desired, nil
	}
	f.selector, f.config = s, bytes.Clone(cfg)
	f.objects.seed("configuration/router.json", cfg)
}

func (f *liveSelectorFixture) selectOK(t *testing.T) Baseline {
	t.Helper()
	start := len(f.objects.calls)
	b, err := f.selector.Select(leaseBudget(t), f.lease)
	if err != nil || b.ID() == "" {
		t.Fatal("valid LIVE selection", err)
	}
	for _, c := range f.objects.calls[start:] {
		if c.method != "GET" && c.name != baselineSelectionName {
			t.Fatalf("unexpected mutation/history: %s %s", c.method, c.name)
		}
	}
	if f.diagnosticSends.Load() != 1 || f.operatorSends.Load() != 1 {
		t.Fatal("selection dispatched test again")
	}
	return b
}
