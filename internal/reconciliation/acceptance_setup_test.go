package reconciliation

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"oci-maintenance-router/internal/notices"
	"oci-maintenance-router/internal/ocinotifications"
	"oci-maintenance-router/internal/pagerduty"
)

func acceptanceRoutes(v map[string]any) {
	destinations, routes := []any{}, []any{}
	for _, owner := range []string{"payments", "video", "blocked", "unrelated"} {
		destinations = append(destinations, map[string]any{"id": owner, "secretId": "ocid1.vaultsecret.oc1.iad." + owner})
		routes = append(routes, map[string]any{"ownerValue": owner, "destinationId": owner})
	}
	v["destinations"], v["routes"] = destinations, routes
}

// Establish readiness through the real operations, not prebuilt receipt records.
// Service callbacks are synthetic; this does not prove managed or human receipt.
func acceptanceSetup(t *testing.T, baseline ...notices.Notice) *acceptanceService {
	t.Helper()
	p, objects, lease := preparationFixture(t, baseline, acceptanceRoutes)
	if err := p.Initialize(leaseBudget(t), lease); err != nil {
		t.Fatal(err)
	}
	prepareSuccessfully(t, p, lease)
	config := liveSchemaConfig(t, "TEST", acceptanceRoutes)
	objects.seed("configuration/router.json", config)
	d, err := NewDiagnosticCoordinator(config, []byte(preparationCatalog), candidateLocation(), strings.Repeat("b", 64), objects, diagnosticVault(t))
	if err != nil {
		t.Fatal(err)
	}
	d.current = func(ctx context.Context, id string) (diagnosticCredential, error) {
		if ctx.Err() != nil || id != "ocid1.vaultsecret.oc1.iad.diagnostic" {
			t.Fatal("diagnostic selected wrong credential or lifetime")
		}
		return diagnosticMaterial{7, func() []byte { return []byte(strings.Repeat("d", 32)) }}, nil
	}
	diagnosticSends := 0
	d.send = func(ctx context.Context, body, key []byte) (pagerduty.Response, error) {
		diagnosticSends++
		store, e := NewDiagnosticStore(objects, lease.manager.environmentID, "installation-1")
		if e != nil {
			t.Fatal(e)
		}
		saved, e := store.Load(ctx, "test-1")
		if e != nil || saved.Record().Snapshot().State != DiagnosticSending || saved.Record().Snapshot().SecretVersion != 7 || !bytes.Equal(saved.Record().Snapshot().PayloadWithoutRoutingKey, body) || !bytes.Equal(key, []byte(strings.Repeat("d", 32))) || !pagerduty.ValidRoutingKey(key) {
			t.Fatal("diagnostic dispatch lacks saved claim/valid key", e)
		}
		return pagerduty.Response{Status: 202}, nil
	}
	diagnostic, err := d.Run(leaseBudget(t), lease)
	if err != nil || diagnostic.Record().Snapshot().State != DiagnosticAccepted || diagnosticSends != 1 {
		t.Fatal("diagnostic setup", err)
	}
	o, err := NewOperatorCoordinator(config, []byte(preparationCatalog), candidateLocation(), objects, operatorClient(t))
	if err != nil {
		t.Fatal(err)
	}
	operatorSends := 0
	o.read = func(context.Context) (operatorObservation, error) {
		return operatorObservation{o.desired, func(ctx context.Context, body []byte) (ocinotifications.Publication, error) {
			operatorSends++
			store, e := NewOperatorChallengeStore(objects, lease.manager.environmentID, "installation-1")
			if e != nil {
				t.Fatal(e)
			}
			saved, e := store.Load(ctx, "operator-1")
			if e != nil || saved.Record().Snapshot().State != OperatorPublishing || !bytes.Equal(saved.Record().Snapshot().Payload, body) {
				t.Fatal("operator publication lacks saved claim", e)
			}
			return ocinotifications.Publication{Status: 200, MessageID: "synthetic-message", MayHaveSent: true}, nil
		}}, nil
	}
	operator, err := o.Run(leaseBudget(t), lease)
	if err != nil || operator.Record().Snapshot().State != OperatorAccepted || operatorSends != 1 {
		t.Fatal("operator setup", err)
	}
	config = liveSchemaConfig(t, "LIVE", acceptanceRoutes, func(v map[string]any) {
		v["diagnostic"].(map[string]any)["receiptCode"] = diagnostic.Record().Snapshot().ConfirmationCode
		v["operatorTest"].(map[string]any)["receiptCode"] = operator.Record().Snapshot().ConfirmationCode
	})
	objects.seed("configuration/router.json", config)
	acceptanceRelease(t, lease)
	s := &acceptanceService{objects: objects, config: config, notices: map[string]notices.Notice{}, owners: map[string]string{}, statuses: map[string]int{}, currentVersion: 7}
	for _, n := range baseline {
		s.notices[n.Key().NoticeID] = n
	}
	// Discard setup handles and only retain the external service's encoded state.
	s.reconstruct(t)
	return s
}

func (s *acceptanceService) recovery(t *testing.T, request map[string]any) {
	t.Helper()
	var v map[string]any
	if err := json.Unmarshal(s.config, &v); err != nil {
		t.Fatal(err)
	}
	v["schemaVersion"], v["recovery"] = 7, request
	var err error
	s.config, err = json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	s.objects.seed("configuration/router.json", s.config)
}
