package deployment

import (
	"crypto/rsa"
	"errors"
	"strings"
	"testing"
	"time"

	"oci-maintenance-router/internal/reconciliation"
)

type unusedPrincipal struct {
	testPrincipal
	reads int
}

func (p *unusedPrincipal) KeyID() (string, error) {
	p.reads++
	return "", errors.New("unexpected credential access")
}
func (p *unusedPrincipal) PrivateRSAKey() (*rsa.PrivateKey, error) {
	p.reads++
	return nil, errors.New("unexpected credential access")
}

func retainedTestFixture(t *testing.T, operator bool) (*Handler, Settings, *operationalStore, string, *unusedPrincipal) {
	t.Helper()
	h, s, objects, environment := operationalFixture(t)
	s = configureOperational(t, h, s, objects, func(v map[string]any) {
		v["mode"] = "TEST"
		v["diagnostic"] = map[string]any{"requestId": "test-1", "secretId": "ocid1.vaultsecret.oc1.iad.diagnostic", "nonProduction": true}
		if operator {
			v["operatorTest"] = map[string]any{"requestId": "operator-1", "subscriptionId": "ocid1.onssubscription.oc1.iad.operator", "endpoint": "operator@example.invalid"}
		}
	})
	principal := &unusedPrincipal{testPrincipal: testPrincipal{tenant: s.TenancyID, region: s.Region}}
	h.connect = func(Settings) (Store, error) { return &managedStore{objects, principal}, nil }
	return h, s, objects, environment, principal
}

func retainDiagnostic(t *testing.T, s Settings, objects *operationalStore, environment string, status int, request ...string) string {
	t.Helper()
	intent := reconciliation.DiagnosticIntent{EnvironmentID: environment, InstallationID: "synthetic-installation", RequestID: "test-1", ReleaseSHA256: s.ReleaseSHA256, CatalogSHA256: s.CatalogSHA256, PagerDutyRegion: "US", SecretID: "ocid1.vaultsecret.oc1.iad.diagnostic", NonProduction: true}
	if len(request) > 0 {
		intent.RequestID = request[0]
	}
	now := time.Now().Add(-time.Second)
	owner, token := strings.Repeat("a", 64), strings.Repeat("b", 64)
	record, err := reconciliation.NewDiagnostic(intent, 7, reconciliation.DiagnosticClaim{OwnerToken: owner, ClaimToken: token, Now: now, ExpiresAt: now.Add(time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	store, err := reconciliation.NewDiagnosticStore(objects, environment, intent.InstallationID)
	if err != nil {
		t.Fatal(err)
	}
	saved, err := store.Create(operationalContext(t), record)
	if err != nil {
		t.Fatal(err)
	}
	if status != 0 {
		next, err := record.Finish(reconciliation.DiagnosticClaimRef{OwnerToken: owner, ClaimToken: token}, status, now.Add(time.Millisecond))
		if err != nil {
			t.Fatal(err)
		}
		saved, err = store.Save(operationalContext(t), saved, next)
		if err != nil {
			t.Fatal(err)
		}
	}
	return saved.Record().Snapshot().ConfirmationCode
}

func retainOperator(t *testing.T, objects *operationalStore, environment string, status int, request ...string) string {
	t.Helper()
	id := "operator-1"
	if len(request) > 0 {
		id = request[0]
	}
	intent, err := reconciliation.NewOperatorIntent(environment, "synthetic-installation", id, reconciliation.OperatorChannel{TopicID: "ocid1.onstopic.oc1.iad.operator", SubscriptionID: "ocid1.onssubscription.oc1.iad.operator", Protocol: "EMAIL", Endpoint: "operator@example.invalid"})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().Add(-time.Second)
	owner, token := strings.Repeat("a", 64), strings.Repeat("b", 64)
	record, err := reconciliation.NewOperatorChallenge(intent, reconciliation.OperatorClaim{OwnerToken: owner, ClaimToken: token, Now: now, ExpiresAt: now.Add(time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	store, err := reconciliation.NewOperatorChallengeStore(objects, environment, intent.InstallationID)
	if err != nil {
		t.Fatal(err)
	}
	saved, err := store.Create(operationalContext(t), record)
	if err != nil {
		t.Fatal(err)
	}
	if status != 0 {
		message := ""
		if status == 200 || status == 202 {
			message = "synthetic-message"
		}
		next, err := record.Finish(reconciliation.OperatorClaimRef{OwnerToken: owner, ClaimToken: token}, status, message, now.Add(time.Millisecond))
		if err != nil {
			t.Fatal(err)
		}
		saved, err = store.Save(operationalContext(t), saved, next)
		if err != nil {
			t.Fatal(err)
		}
	}
	return saved.Record().Snapshot().ConfirmationCode
}
