package reconciliation

import (
	"encoding/json"
	"oci-maintenance-router/internal/noticeledger"
	"strings"
	"testing"
	"time"
)

func reviewedRecovery(t *testing.T, r noticeledger.Record, kind noticeledger.ActionKind, target string) map[string]any {
	t.Helper()
	if r.Snapshot().SchemaVersion == 2 {
		var err error
		r, err = r.MigrateRecovery(time.Now())
		if err != nil {
			t.Fatal(err)
		}
	}
	generation := r.Snapshot().Revisions[0].RoutingGeneration
	component := ""
	q := map[string]any{"requestId": "recover-1", "noticeId": r.Snapshot().NoticeID,
		"kind": string(kind), "targetId": target, "routingGeneration": generation}
	if kind == noticeledger.RetryBatch || kind == noticeledger.ApproveRevision {
		component = savedMembership(r)
		q["componentFingerprint"] = component
	}
	hash, err := r.ReviewFingerprint(kind, target, generation, component)
	if err != nil {
		t.Fatal(err)
	}
	q["reviewFingerprint"] = hash
	return q
}

func recoveryInput(t *testing.T, f *processingFixture, mode string, q map[string]any) []byte {
	t.Helper()
	var v map[string]any
	if err := json.Unmarshal(f.live.config, &v); err != nil {
		t.Fatal(err)
	}
	v["schemaVersion"], v["mode"] = 7, mode
	if q != nil {
		v["recovery"] = q
	}
	body, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	f.live.objects.seed("configuration/router.json", body)
	return body
}

func recoveryOnlyProcessor(t *testing.T, f *processingFixture, mode string, q map[string]any) *MaintenanceProcessor {
	t.Helper()
	p, err := NewMaintenanceProcessor(recoveryInput(t, f, mode, q), []byte(preparationCatalog), candidateLocation(), strings.Repeat("b", 64), nil, f.live.objects, nil, nil, nil)
	if err != nil || p == nil {
		t.Fatal("explicit recovery without sending clients rejected", err)
	}
	return p
}

func recoveryActions(t *testing.T, f *processingFixture, id string) []noticeledger.RecoveryAction {
	t.Helper()
	s := f.saved(t, id).Snapshot()
	if s.SchemaVersion != 3 || s.Recovery == nil {
		t.Fatal("recovery history not persisted")
	}
	return s.Recovery.Actions
}
