package deployment

import (
	"context"
	"oci-maintenance-router/internal/configuration"
	"oci-maintenance-router/internal/ocisecrets"
	"oci-maintenance-router/internal/reconciliation"
	"testing"
)

func TestSetupProjectionRefusesIncompleteSuccessfulResults(t *testing.T) {
	h, s, objects, env, principal := retainedTestFixture(t, false)
	_ = h
	retainDiagnostic(t, s, objects, env, 202)
	p, failure := loadPinned(operationalContext(t), s, objects)
	if failure.Status != "" {
		t.Fatal(failure)
	}
	vault, err := ocisecrets.New(s.Region, p.catalog.Regions[s.Region], principal)
	if err != nil {
		t.Fatal(err)
	}
	c, err := reconciliation.NewDiagnosticCoordinator(p.configBytes, p.catalogBytes, configuration.LedgerLocation{Namespace: s.Namespace, Bucket: s.Bucket}, s.ReleaseSHA256, objects, vault)
	if err != nil {
		t.Fatal(err)
	}
	m, err := reconciliation.NewLeaseManager(objects, env)
	if err != nil {
		t.Fatal(err)
	}
	lease, err := m.Acquire(operationalContext(t))
	if err != nil || lease == nil {
		t.Fatal(err)
	}
	defer lease.Release(operationalContext(t))
	diagnostic, err := c.Run(operationalContext(t), lease)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		saved reconciliation.SetupResult
		code  string
	}{
		{reconciliation.SetupResult{}, "diagnostic_not_accepted"},
		{reconciliation.SetupResult{Diagnostic: diagnostic}, "operator_not_accepted"},
	} {
		var result OperationalResult
		err := setupSendOutcome(context.Background(), tc.saved, nil, &result)
		if err == nil || operationCode(context.Background(), err) != tc.code || receiptVerified(t, result.Diagnostic) || receiptVerified(t, result.Operator) {
			t.Fatal("incomplete result reported setup success", result, err)
		}
	}
	if principal.reads != 0 {
		t.Fatal("retained projection test accessed credentials")
	}
}
