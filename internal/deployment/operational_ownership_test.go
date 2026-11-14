package deployment

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"oci-maintenance-router/internal/configuration"
	"oci-maintenance-router/internal/ocistorage"
	"oci-maintenance-router/internal/reconciliation"
)

func operationalFixture(t *testing.T) (*Handler, Settings, *operationalStore, string) {
	t.Helper()
	s, stored := fixture(t)
	var data map[string]any
	if err := json.Unmarshal(stored.config, &data); err != nil {
		t.Fatal(err)
	}
	data["schemaVersion"], data["mode"] = 7, "PREVIEW"
	data["installationId"] = "synthetic-installation"
	data["preparation"] = map[string]any{"initialize": true}
	stored.config, _ = json.Marshal(data)
	s.ConfigurationSHA256 = digestFixture(stored.config)
	objects := &operationalStore{pinned: stored, objects: map[string]ocistorage.Object{}, history: map[string]bool{}}
	raw, _ := json.Marshal(s)
	h := New(func(string) string { return string(raw) })
	h.connect = func(Settings) (Store, error) { return objects, nil }
	catalog, err := configuration.DecodeCatalog(stored.catalog)
	if err != nil {
		t.Fatal(err)
	}
	c, err := configuration.Decode(stored.config, catalog)
	if err != nil {
		t.Fatal(err)
	}
	environment, err := c.EnvironmentID(configuration.LedgerLocation{Namespace: s.Namespace, Bucket: s.Bucket})
	if err != nil {
		t.Fatal(err)
	}
	return h, s, objects, environment
}

func operationalContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	t.Cleanup(cancel)
	return ctx
}

func TestOperationalInvocationCoalescesWithoutClaimingVerification(t *testing.T) {
	for _, operation := range []string{"deployment-check", "reconcile"} {
		t.Run(operation, func(t *testing.T) {
			h, s, objects, environment := operationalFixture(t)
			manager, err := reconciliation.NewLeaseManager(objects, environment)
			if err != nil {
				t.Fatal(err)
			}
			lease, err := manager.Acquire(operationalContext(t))
			if err != nil || lease == nil {
				t.Fatal("fixture ownership", err)
			}
			before := objects.version
			body := invocation(s)
			if operation == "reconcile" {
				body = `{"operation":"reconcile"}`
			}
			out := httptest.NewRecorder()
			h.Serve(operationalContext(t), strings.NewReader(body), out)
			var result struct {
				ResponseVersion int    `json:"responseVersion"`
				Status          string `json:"status"`
				ScanCompleted   bool   `json:"scanCompleted"`
			}
			if json.Unmarshal(out.Body.Bytes(), &result) != nil || result.ResponseVersion != 2 || result.Status != "coalesced" || result.ScanCompleted || objects.version != before {
				t.Fatalf("overlap reported readiness or changed owner: %s; versions=%d/%d", out.Body, before, objects.version)
			}
			if err := lease.Check(operationalContext(t), time.Second); err != nil {
				t.Fatal("invocation released another owner", err)
			}
		})
	}
}
