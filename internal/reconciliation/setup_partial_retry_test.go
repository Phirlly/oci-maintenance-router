package reconciliation

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestSetupPartialOperatorFailureRetriesOnlyUnconsumedChannel(t *testing.T) {
	f := setupRuntimeFixtureFor(t)
	read := f.coordinator.operator.read
	f.coordinator.operator.read = func(context.Context) (operatorObservation, error) {
		return operatorObservation{}, errors.New("private channel failure")
	}
	got, err := f.coordinator.Run(leaseBudget(t), f.lease)
	if err == nil || got.Diagnostic.Record().Snapshot().State != DiagnosticAccepted || got.Operator.Record().Snapshot().State != "" || f.diagnosticSends != 1 || f.operatorSends != 0 {
		t.Fatal("partial outcome lost", err)
	}
	f.coordinator.operator.read = read
	got, err = f.coordinator.Run(leaseBudget(t), f.lease)
	if err != nil || got.Diagnostic.Dispatched() || !got.Operator.Dispatched() || f.diagnosticSends != 1 || f.operatorSends != 1 {
		t.Fatal("retry resent consumed channel or skipped remaining channel", err)
	}
}

func TestSetupConfirmationRejectsStaleIntentAndRecordFailure(t *testing.T) {
	for _, kind := range []string{"region", "catalog", "channel", "diagnostic-missing", "operator-missing", "operator-corrupt"} {
		t.Run(kind, func(t *testing.T) {
			f := setupRuntimeFixtureFor(t)
			var err error
			f.result, err = f.coordinator.Run(leaseBudget(t), f.lease)
			if err != nil {
				t.Fatal(err)
			}
			f.configure(t, "CONFIRM", 1, func(v map[string]any) {
				if kind == "region" {
					v["pagerDutyRegion"] = "EU"
				}
				if kind == "channel" {
					v["operatorChannel"].(map[string]any)["endpoint"] = "changed@example.invalid"
				}
			})
			if kind == "catalog" {
				catalog := []byte(preparationCatalog + "\n")
				f.objects.seed("configuration/catalog.json", catalog)
				f.coordinator, err = NewSetupCoordinator(f.config, catalog, candidateLocation(), strings.Repeat("b", 64), f.objects, operatorClient(t), nil)
				if err != nil {
					t.Fatal(err)
				}
			}
			d, o := setupRequestIDs(f.lease.manager.environmentID, "installation-1", 1)
			if kind == "diagnostic-missing" {
				delete(f.objects.objects, "control/diagnostics/installation-1/"+d+".json")
			}
			if kind == "operator-missing" {
				delete(f.objects.objects, "control/operator-challenges/installation-1/"+o+".json")
			}
			if kind == "operator-corrupt" {
				f.objects.seed("control/operator-challenges/installation-1/"+o+".json", []byte("invalid"))
			}
			got, err := f.coordinator.Run(leaseBudget(t), f.lease)
			if err == nil || got.Confirmation.Confirmation.ID() != "" || f.diagnosticSends != 1 || f.operatorSends != 1 {
				t.Fatal("stale confirmation granted authority", err)
			}
		})
	}
}

func TestSetupReadinessRejectsNewConfigurationChannelOrCatalog(t *testing.T) {
	for _, kind := range []string{"region", "catalog", "channel"} {
		t.Run(kind, func(t *testing.T) {
			f := setupConfirmedRuntime(t)
			cfg := setupRuntimeConfig(t, "LIVE", "NONE", 0, func(v map[string]any) {
				if kind == "region" {
					v["pagerDutyRegion"] = "EU"
				}
				if kind == "channel" {
					v["operatorChannel"].(map[string]any)["endpoint"] = "changed@example.invalid"
				}
			})
			catalog := []byte(preparationCatalog)
			if kind == "catalog" {
				catalog = append(catalog, '\n')
			}
			f.objects.seed("configuration/router.json", cfg)
			f.objects.seed("configuration/catalog.json", catalog)
			v, err := newSetupReadiness(cfg, catalog, candidateLocation(), f.objects, operatorClient(t))
			if err != nil {
				t.Fatal(err)
			}
			v.read = f.coordinator.read
			if err = v.verify(leaseBudget(t), f.lease, strings.Repeat("b", 64)); err == nil {
				t.Fatal("changed prerequisites reused confirmation")
			}
		})
	}
}
