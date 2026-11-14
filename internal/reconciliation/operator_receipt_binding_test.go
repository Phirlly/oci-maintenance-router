package reconciliation

import (
	"bytes"
	"sync"
	"testing"
)

func TestOperatorReceiptRejectsChangedIntent(t *testing.T) {
	for _, kind := range []string{"topic", "subscription", "endpoint", "request", "installation", "environment"} {
		t.Run(kind, func(t *testing.T) {
			f := operatorCoordinatorFixtureFor(t)
			r := f.seed(t, OperatorAccepted, false)
			cfg := operatorReceiptConfig(t, r, func(c map[string]any) {
				o := c["operatorTest"].(map[string]any)
				switch kind {
				case "topic":
					c["operatorTopicId"] = "ocid1.onstopic.oc1.iad.other"
				case "subscription":
					o["subscriptionId"] = "ocid1.onssubscription.oc1.iad.other"
				case "endpoint":
					o["endpoint"] = "Other@example.invalid"
				case "request":
					o["requestId"] = "operator-2"
				case "installation":
					c["installationId"] = "installation-2"
				case "environment":
					c["tenancyId"] = "ocid1.tenancy.oc1..other"
				}
			})
			v := operatorReceiptVerifier(t, f, cfg)
			body, _ := r.Encode()
			f.objects.seed(v.store.name(v.input.intent.RequestID), body)
			lease := f.lease
			want := "identity_conflict"
			if kind == "request" || kind == "installation" || kind == "environment" {
				want = "invalid_record"
			}
			if kind == "environment" {
				delete(f.objects.objects, workerLeaseName)
				m, err := NewLeaseManager(f.objects, v.input.intent.EnvironmentID)
				if err != nil {
					t.Fatal(err)
				}
				lease = acquireLease(t, m, leaseBudget(t))
			}
			before := len(f.objects.calls)
			got, err := v.Verify(leaseBudget(t), lease)
			operatorReceiptFailure(t, got, err, want)
			operatorReceiptReadOnly(t, f, before)
			if f.reads.Load() != 0 || f.sends.Load() != 0 {
				t.Fatal("old identity reached channel or publisher")
			}
		})
	}
}

func TestOperatorReceiptSurvivesIndependentEditsOnlyWithCurrentPins(t *testing.T) {
	for _, kind := range []string{"route", "scope", "ownership", "destination", "presentation", "catalog"} {
		f := operatorCoordinatorFixtureFor(t)
		r := f.seed(t, OperatorAccepted, false)
		old := operatorReceiptVerifier(t, f, operatorReceiptConfig(t, r))
		cfg := operatorReceiptConfig(t, r, func(c map[string]any) {
			switch kind {
			case "route":
				c["routes"].([]any)[0].(map[string]any)["ownerValue"] = "other"
			case "scope":
				c["compartmentIds"] = []string{"ocid1.compartment.oc1..other"}
			case "ownership":
				c["ownership"].(map[string]any)["key"] = "team"
			case "destination":
				c["destinations"].([]any)[0].(map[string]any)["secretId"] = "ocid1.vaultsecret.oc1.iad.other"
			case "presentation":
				c["environmentName"] = "other"
			}
		})
		current := operatorReceiptVerifier(t, f, cfg)
		if kind == "catalog" {
			cat := append([]byte(preparationCatalog), '\n')
			v, err := NewOperatorReceiptVerifier(cfg, cat, candidateLocation(), f.objects, operatorClient(t))
			if err != nil {
				t.Fatal(err)
			}
			v.read = current.read
			current = v
			f.objects.seed("configuration/catalog.json", cat)
		}
		before := len(f.objects.calls)
		got, err := old.Verify(leaseBudget(t), f.lease)
		operatorReceiptFailure(t, got, err, "configuration_changed")
		got, err = current.Verify(leaseBudget(t), f.lease)
		if err != nil || !got.Record().Matches(r.Snapshot().Intent) || f.reads.Load() != 1 || f.sends.Load() != 0 {
			t.Fatal("independent edit invalidated current attestation", kind, err)
		}
		operatorReceiptReadOnly(t, f, before)
	}
}

func TestOperatorReceiptConcurrentVerificationIsReadOnly(t *testing.T) {
	f := operatorCoordinatorFixtureFor(t)
	r := f.seed(t, OperatorAccepted, false)
	v := operatorReceiptVerifier(t, f, operatorReceiptConfig(t, r))
	before := len(f.objects.calls)
	want, _ := r.Encode()
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			got, err := v.Verify(leaseBudget(t), f.lease)
			body, _ := got.Record().Encode()
			if err != nil || !bytes.Equal(body, want) {
				t.Error("concurrent verification changed evidence", err)
			}
			clear(got.Record().Snapshot().Payload)
		})
	}
	wg.Wait()
	operatorReceiptReadOnly(t, f, before)
	if f.reads.Load() != 8 || f.sends.Load() != 0 {
		t.Fatal("verification skipped current channel or published")
	}
}
