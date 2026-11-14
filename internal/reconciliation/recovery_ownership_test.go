package reconciliation

import (
	"context"
	"oci-maintenance-router/internal/delivery"
	"oci-maintenance-router/internal/noticeledger"
	"oci-maintenance-router/internal/routing"
	"testing"
	"time"
)

func TestRecoveryRetryAllowsCompatibleRouteEditButNeverRetargets(t *testing.T) {
	for _, changedSecret := range []bool{false, true} {
		n := processingNotice(t, 1, processingDB)
		f := processingFixtureFor(t, n)
		old := f.seedBatch(t, n, time.Now().Add(-time.Minute), delivery.Prepared, true)
		edit := func(v map[string]any) {
			if changedSecret {
				v["destinations"].([]any)[0].(map[string]any)["secretId"] = "ocid1.vaultsecret.oc1.iad.replacement"
			} else {
				v["routes"] = append(v["routes"].([]any), map[string]any{"ownerValue": "extra", "destinationId": "payments"})
			}
		}
		f.configure(t, edit)
		metadata, err := f.p.mysql["us-ashburn-1"](leaseBudget(t), processingDB)
		if err != nil {
			t.Fatal(err)
		}
		current, err := f.p.observer.Observe(old, n, true, map[string]routing.Lookup{processingDB: {State: routing.Found, Metadata: metadata}}, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		body, _ := current.Encode()
		f.live.objects.seed(noticeName(n.Key().NoticeID), body)
		q := reviewedRecovery(t, current, noticeledger.RetryBatch, old.Snapshot().Batches[0].Message.BatchID)
		f.configure(t, func(v map[string]any) { edit(v); v["schemaVersion"], v["recovery"] = 7, q })
		s := f.run(t)
		want := string(noticeledger.RecoveryAccepted)
		if changedSecret {
			want = string(noticeledger.RecoveryRejected)
		}
		if s.Recovery == nil || s.Recovery.Status != want || (f.sent.Load() == 1) == changedSecret || f.current.Load() != 0 {
			t.Fatal("wrong compatibility disposition", changedSecret, s)
		}
		b := f.saved(t, n.Key().NoticeID).Snapshot().Batches[0]
		if b.Message.RoutingGeneration != old.Snapshot().Batches[0].Message.RoutingGeneration || b.Message.SecretID != old.Snapshot().Batches[0].Message.SecretID {
			t.Fatal("saved identity retargeted")
		}
	}
}

func TestRecoveryRetryRequiresCurrentOwnershipAndExactVersion(t *testing.T) {
	for _, fault := range []string{"owner", "scope", "missing-version", "wrong-version", "invalid-key"} {
		n := processingNotice(t, 1, processingDB)
		f := processingFixtureFor(t, n)
		old := f.seedBatch(t, n, time.Now().Add(-time.Minute), delivery.Prepared, true)
		q := reviewedRecovery(t, old, noticeledger.RetryBatch, old.Snapshot().Batches[0].Message.BatchID)
		f.configure(t, func(v map[string]any) { v["schemaVersion"], v["recovery"] = 7, q })
		lookup := f.p.mysql["us-ashburn-1"]
		f.p.mysql["us-ashburn-1"] = func(ctx context.Context, id string) (routing.Metadata, error) {
			m, err := lookup(ctx, id)
			if fault == "owner" {
				m.FreeformTags["owner"] = "other"
			}
			if fault == "scope" {
				m.CompartmentID = chain("ocid1.compartment.oc1..outside")
			}
			return m, err
		}
		version := f.p.version
		f.p.version = func(ctx context.Context, id string, v int64) (maintenanceCredential, error) {
			key, err := version(ctx, id, v)
			switch fault {
			case "missing-version":
				return nil, nil
			case "wrong-version":
				return diagnosticMaterial{8, key.Bytes}, nil
			case "invalid-key":
				return diagnosticMaterial{7, func() []byte { return []byte("bad") }}, nil
			}
			return key, err
		}
		s := f.run(t)
		want := "secret_unavailable"
		if fault == "owner" || fault == "scope" {
			want = "ownership_changed"
		}
		if s.Recovery == nil || s.Recovery.Reason != want || f.sent.Load() != 0 || f.current.Load() != 0 {
			t.Fatal("unsafe retry prerequisite accepted", fault, s)
		}
	}
}
