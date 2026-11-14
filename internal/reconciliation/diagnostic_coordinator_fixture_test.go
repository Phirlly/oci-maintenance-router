package reconciliation

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/oracle/oci-go-sdk/v65/common"
	"oci-maintenance-router/internal/configuration"
	"oci-maintenance-router/internal/ocisecrets"
	"oci-maintenance-router/internal/ocistorage"
	"oci-maintenance-router/internal/pagerduty"
)

type diagnosticMaterial struct {
	version int64
	content func() []byte
}

func (s diagnosticMaterial) Version() int64 { return s.version }
func (s diagnosticMaterial) Bytes() []byte  { return s.content() }

type diagnosticCoordinatorFixture struct {
	coordinator  *DiagnosticCoordinator
	objects      *preparationObjects
	lease        *Lease
	reads, sends atomic.Int32
}

func diagnosticConfig(t *testing.T, changes ...func(map[string]any)) []byte {
	t.Helper()
	return preparationConfig(t, append([]func(map[string]any){func(c map[string]any) {
		c["schemaVersion"], c["mode"] = 3, "TEST"
		c["diagnostic"] = map[string]any{"requestId": "test-1", "secretId": "ocid1.vaultsecret.oc1.iad.diagnostic", "nonProduction": true}
	}}, changes...)...)
}
func diagnosticVault(t *testing.T) *ocisecrets.Client {
	t.Helper()
	v, err := ocisecrets.New("us-ashburn-1", "iad", common.NewRawConfigurationProvider("synthetic", "synthetic", "us-ashburn-1", "synthetic", "synthetic", nil))
	if err != nil {
		t.Fatal(err)
	}
	return v
}
func coordinatorFixture(t *testing.T, origins ...context.Context) *diagnosticCoordinatorFixture {
	t.Helper()
	f := &diagnosticCoordinatorFixture{objects: &preparationObjects{discoveryObjects: &discoveryObjects{objects: map[string]ocistorage.Object{}}}}
	cfg := diagnosticConfig(t)
	f.objects.seed("configuration/router.json", cfg)
	f.objects.seed("configuration/catalog.json", []byte(preparationCatalog))
	var err error
	f.coordinator, err = NewDiagnosticCoordinator(cfg, []byte(preparationCatalog), candidateLocation(), strings.Repeat("b", 64), f.objects, diagnosticVault(t))
	if err != nil {
		t.Fatal("valid coordinator rejected", err)
	}
	catalog, _ := configuration.DecodeCatalog([]byte(preparationCatalog))
	config, _ := configuration.Decode(cfg, catalog)
	env, _ := config.EnvironmentID(candidateLocation())
	m, err := NewLeaseManager(f.objects, env)
	if err != nil {
		t.Fatal(err)
	}
	origin := leaseBudget(t)
	if len(origins) > 0 {
		origin = origins[0]
	}
	f.lease = acquireLease(t, m, origin)
	f.coordinator.current = func(_ context.Context, id string) (diagnosticCredential, error) {
		f.reads.Add(1)
		if id != "ocid1.vaultsecret.oc1.iad.diagnostic" {
			t.Error("wrong secret reference")
		}
		return diagnosticMaterial{7, func() []byte { return []byte("synthetic-key") }}, nil
	}
	f.coordinator.send = func(ctx context.Context, body, key []byte) (pagerduty.Response, error) {
		f.sends.Add(1)
		got := f.saved(t).Snapshot()
		if got.State != DiagnosticSending || got.SecretVersion != 7 || !bytes.Equal(body, got.PayloadWithoutRoutingKey) || string(key) != "synthetic-key" || ctx.Err() != nil {
			t.Error("send did not use the confirmed claim and selected credential")
		}
		return pagerduty.Response{Status: 202}, nil
	}
	return f
}
func (f *diagnosticCoordinatorFixture) saved(t *testing.T) DiagnosticRecord {
	t.Helper()
	f.objects.mu.Lock()
	b := bytes.Clone(f.objects.objects[diagnosticObjectName].Body)
	f.objects.mu.Unlock()
	r, err := DecodeDiagnostic(b, f.coordinator.intent.EnvironmentID, "installation-1", "test-1")
	if err != nil {
		t.Fatal(err)
	}
	return r
}
func (f *diagnosticCoordinatorFixture) seed(t *testing.T, state DiagnosticState, expired bool) DiagnosticRecord {
	t.Helper()
	now := f.lease.manager.now().UTC().Truncate(time.Millisecond)
	start := now.Add(-time.Minute)
	if expired {
		start = now.Add(-11 * time.Minute)
	}
	r, err := NewDiagnostic(f.coordinator.intent, 9, DiagnosticClaim{strings.Repeat("c", 64), strings.Repeat("d", 64), start, start.Add(10 * time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	if state != DiagnosticSending {
		status := 202
		if state == DiagnosticReview {
			status = 400
		}
		r, err = r.Finish(DiagnosticClaimRef{strings.Repeat("c", 64), strings.Repeat("d", 64)}, status, start.Add(time.Second))
		if err != nil {
			t.Fatal(err)
		}
	}
	b, _ := r.Encode()
	f.objects.seed(diagnosticObjectName, b)
	return r
}
func diagnosticRunFailure(t *testing.T, err error, code string) *DiagnosticRunError {
	t.Helper()
	var e *DiagnosticRunError
	if !errors.As(err, &e) || e.Code != code {
		t.Fatalf("error=%v, want %s", err, code)
	}
	return e
}
func diagnosticReconfigure(t *testing.T, f *diagnosticCoordinatorFixture, change func(map[string]any)) {
	t.Helper()
	cfg := diagnosticConfig(t, change)
	d, err := NewDiagnosticCoordinator(cfg, []byte(preparationCatalog), candidateLocation(), strings.Repeat("b", 64), f.objects, diagnosticVault(t))
	if err != nil {
		t.Fatal(err)
	}
	d.current, d.send = f.coordinator.current, f.coordinator.send
	f.coordinator = d
	f.objects.seed("configuration/router.json", cfg)
}
