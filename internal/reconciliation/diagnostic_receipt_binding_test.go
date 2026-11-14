package reconciliation

import (
	"bytes"
	"strings"
	"sync"
	"testing"
)

func TestDiagnosticReceiptRejectsChangedTestIdentity(t *testing.T) {
	for _, kind := range []string{"release", "catalog", "region", "secret", "request", "installation", "environment"} {
		t.Run(kind, func(t *testing.T) {
			f := coordinatorFixture(t)
			r := f.seed(t, DiagnosticAccepted, false)
			cat, release := []byte(preparationCatalog), strings.Repeat("b", 64)
			cfg := receiptConfig(t, r, func(c map[string]any) {
				switch kind {
				case "region":
					c["pagerDutyRegion"] = "EU"
				case "secret":
					c["diagnostic"].(map[string]any)["secretId"] = "ocid1.vaultsecret.oc1.iad.other"
				case "request":
					c["diagnostic"].(map[string]any)["requestId"] = "test-2"
				case "installation":
					c["installationId"] = "installation-2"
				case "environment":
					c["tenancyId"] = "ocid1.tenancy.oc1..other"
				}
			})
			if kind == "release" {
				release = strings.Repeat("c", 64)
			}
			if kind == "catalog" {
				cat = append(cat, '\n')
			}
			v, err := NewDiagnosticReceiptVerifier(cfg, cat, candidateLocation(), release, f.objects)
			if err != nil {
				t.Fatal(err)
			}
			f.objects.seed("configuration/router.json", cfg)
			f.objects.seed("configuration/catalog.json", cat)
			// Place the old valid record at the new requested path to prove its
			// own saved identity is checked, not just the location or code.
			body, _ := r.Encode()
			f.objects.seed(v.store.name(v.input.intent.RequestID), body)
			lease := f.lease
			want := "identity_conflict"
			if kind == "request" || kind == "installation" || kind == "environment" {
				want = "invalid_record"
			}
			if kind == "environment" {
				// Synthetic new environment, genuinely acquired independent lease.
				delete(f.objects.objects, workerLeaseName)
				m, err := NewLeaseManager(f.objects, v.input.intent.EnvironmentID)
				if err != nil {
					t.Fatal(err)
				}
				lease = acquireLease(t, m, leaseBudget(t))
			}
			before := len(f.objects.calls)
			got, err := v.Verify(leaseBudget(t), lease)
			receiptFailure(t, got, err, want)
			receiptReadOnly(t, f, before)
		})
	}
}

func TestDiagnosticReceiptSurvivesOrdinaryRoutingEditsOnlyWithCurrentPins(t *testing.T) {
	for _, kind := range []string{"route", "ownership", "scope", "destination", "presentation"} {
		f := coordinatorFixture(t)
		r := f.seed(t, DiagnosticAccepted, false)
		old := receiptVerifier(t, f, receiptConfig(t, r))
		cfg := receiptConfig(t, r, func(c map[string]any) {
			switch kind {
			case "route":
				c["routes"].([]any)[0].(map[string]any)["ownerValue"] = "other"
			case "ownership":
				c["ownership"].(map[string]any)["key"] = "other-owner"
			case "scope":
				c["compartmentIds"] = []string{"ocid1.compartment.oc1..other"}
			case "destination":
				c["destinations"].([]any)[0].(map[string]any)["secretId"] = "ocid1.vaultsecret.oc1.iad.other"
			case "presentation":
				c["environmentName"] = "other"
			}
		})
		current := receiptVerifier(t, f, cfg)
		before := len(f.objects.calls)
		got, err := old.Verify(leaseBudget(t), f.lease)
		receiptFailure(t, got, err, "configuration_changed")
		got, err = current.Verify(leaseBudget(t), f.lease)
		if err != nil || !got.Record().Matches(r.Snapshot().Intent) {
			t.Fatal("routing edit invalidated independent test", kind, err)
		}
		receiptReadOnly(t, f, before)
	}
}

func TestDiagnosticReceiptConcurrentVerificationIsReadOnlyAndIndependent(t *testing.T) {
	f := coordinatorFixture(t)
	r := f.seed(t, DiagnosticAccepted, false)
	v := receiptVerifier(t, f, receiptConfig(t, r))
	before := len(f.objects.calls)
	want, _ := r.Encode()
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			got, err := v.Verify(leaseBudget(t), f.lease)
			body, _ := got.Record().Encode()
			if err != nil || !bytes.Equal(body, want) {
				t.Error("concurrent verifier changed evidence", err)
			}
			clear(got.Record().Snapshot().PayloadWithoutRoutingKey)
		})
	}
	wg.Wait()
	receiptReadOnly(t, f, before)
	if f.reads.Load() != 0 || f.sends.Load() != 0 {
		t.Fatal("verification accessed credential/transport")
	}
}
