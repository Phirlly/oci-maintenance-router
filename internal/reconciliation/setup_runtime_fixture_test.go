package reconciliation

import (
	"context"
	"strings"
	"testing"

	"oci-maintenance-router/internal/ocinotifications"
	"oci-maintenance-router/internal/ocisecrets"
	"oci-maintenance-router/internal/pagerduty"
)

func setupRuntimeConfig(t *testing.T, mode, action string, attempt int64, changes ...func(map[string]any)) []byte {
	t.Helper()
	return preparationConfig(t, append([]func(map[string]any){func(v map[string]any) {
		v["schemaVersion"], v["mode"] = 8, mode
		channel := operatorChannelFixture()
		v["operatorChannel"] = map[string]any{"subscriptionId": channel.SubscriptionID, "endpoint": channel.Endpoint}
		s := map[string]any{"action": action}
		if action != "NONE" {
			s["attempt"] = attempt
		}
		if action == "SEND_TEST" {
			s["secretId"], s["nonProduction"] = "ocid1.vaultsecret.oc1.iad.diagnostic", true
		}
		v["setup"] = s
	}}, changes...)...)
}

type setupRuntimeFixture struct {
	objects                        *preparationObjects
	lease                          *Lease
	coordinator                    *SetupCoordinator
	config                         []byte
	diagnosticSends, operatorSends int
	result                         SetupResult
}

func setupRuntimeFixtureFor(t *testing.T) *setupRuntimeFixture {
	t.Helper()
	p, objects, lease := preparationFixture(t, nil)
	if err := p.Initialize(leaseBudget(t), lease); err != nil {
		t.Fatal(err)
	}
	prepareSuccessfully(t, p, lease)
	f := &setupRuntimeFixture{objects: objects, lease: lease}
	f.configure(t, "SEND_TEST", 1)
	return f
}

func (f *setupRuntimeFixture) configure(t *testing.T, action string, attempt int64, changes ...func(map[string]any)) {
	t.Helper()
	if action == "CONFIRM" {
		changes = append([]func(map[string]any){func(v map[string]any) {
			s := v["setup"].(map[string]any)
			s["pagerDutyCode"] = f.result.Diagnostic.Record().Snapshot().ConfirmationCode
			s["operatorCode"] = f.result.Operator.Record().Snapshot().ConfirmationCode
		}}, changes...)
	}
	cfg := setupRuntimeConfig(t, "TEST", action, attempt, changes...)
	f.objects.seed("configuration/router.json", cfg)
	var vault *ocisecrets.Client
	if action == "SEND_TEST" {
		vault = diagnosticVault(t)
	}
	c, err := NewSetupCoordinator(cfg, []byte(preparationCatalog), candidateLocation(), strings.Repeat("b", 64), f.objects, operatorClient(t), vault)
	if err != nil {
		t.Fatal(err)
	}
	if action == "SEND_TEST" {
		c.diagnostic.current = func(context.Context, string) (diagnosticCredential, error) {
			f.assertSelected(t, attempt)
			return diagnosticMaterial{7, func() []byte { return []byte("synthetic-key") }}, nil
		}
		c.diagnostic.send = func(context.Context, []byte, []byte) (pagerduty.Response, error) {
			f.assertSelected(t, attempt)
			f.diagnosticSends++
			return pagerduty.Response{Status: 202}, nil
		}
		c.operator.read = func(context.Context) (operatorObservation, error) {
			return operatorObservation{c.operator.desired, func(context.Context, []byte) (ocinotifications.Publication, error) {
				f.assertSelected(t, attempt)
				f.operatorSends++
				return ocinotifications.Publication{Status: 200, MessageID: "synthetic-message", MayHaveSent: true}, nil
			}}, nil
		}
	}
	c.read = func(context.Context) (OperatorChannel, error) { return operatorChannelFixture(), nil }
	f.config, f.coordinator = cfg, c
}

func (f *setupRuntimeFixture) assertSelected(t *testing.T, attempt int64) {
	t.Helper()
	s, _ := newSetupCurrentStore(f.objects, f.lease.manager.environmentID, "installation-1")
	h, missing, err := s.load(leaseBudget(t))
	if err != nil || missing || h.record.data.Attempt != attempt {
		t.Fatal("test called before durable selection", err)
	}
}
