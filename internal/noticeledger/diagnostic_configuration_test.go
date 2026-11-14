package noticeledger

import (
	"oci-maintenance-router/internal/delivery"
	"reflect"
	"testing"
	"time"
)

func diagnosticConfig(v map[string]any) {
	preparationConfig(v)
	v["schemaVersion"] = 3
	v["diagnostic"] = map[string]any{"requestId": "test-1", "secretId": "ocid1.vaultsecret.oc1.iad.test", "nonProduction": true}
}

func TestDiagnosticConfigurationPreservesSavedMaintenanceWork(t *testing.T) {
	for _, state := range []delivery.State{delivery.Prepared, delivery.RetryWait, delivery.APIAccepted} {
		src := source(db)
		r := prepare(t, observed(t, src, db), preparations(t, src, db)...)
		if state != delivery.Prepared {
			var ref delivery.AttemptRef
			r, ref = begin(t, r, r.Snapshot().Batches[0].Message.BatchID, clock())
			status, retryAt := 503, clock().Add(time.Minute)
			if state == delivery.APIAccepted {
				status, retryAt = 202, time.Time{}
			}
			var err error
			r, err = r.Finish(ref, status, clock(), retryAt)
			if err != nil {
				t.Fatal(err)
			}
		}
		before := r.Snapshot()
		for _, change := range []func(map[string]any){diagnosticConfig, func(v map[string]any) {
			diagnosticConfig(v)
			v["diagnostic"] = map[string]any{"requestId": "new-test", "secretId": "ocid1.vaultsecret.oc1.iad.other", "nonProduction": true}
		}} {
			next, err := observer(t, change).Observe(r, normalized(t, src), false, lookups(db), clock())
			if err != nil {
				t.Fatal(err)
			}
			after := next.Snapshot()
			if after.HoldReason != "" || after.Revisions[0].RoutingGeneration != before.Revisions[0].RoutingGeneration || !reflect.DeepEqual(after.Batches, before.Batches) {
				t.Fatal("test control changed saved maintenance identity")
			}
			body, err := next.Encode()
			if err != nil {
				t.Fatal(err)
			}
			restored, err := Decode(body, after.EnvironmentID, after.NoticeID)
			if err != nil || !reflect.DeepEqual(restored.Snapshot().Batches, before.Batches) {
				t.Fatal("restart lost saved payload/pins/receipts", err)
			}
		}
	}
}
func TestDiagnosticSchemaRealRoutingEditStillHoldsWork(t *testing.T) {
	src := source(db)
	r := prepare(t, observed(t, src, db), preparations(t, src, db)...)
	next, err := observer(t, func(v map[string]any) { diagnosticConfig(v); v["pagerDutyRegion"] = "EU" }).Observe(r, normalized(t, src), false, lookups(db), clock())
	if err != nil {
		t.Fatal(err)
	}
	b := next.Snapshot().Batches[0]
	if b.State != delivery.ReviewRequired || b.Reason != delivery.ReasonConfigurationChanged {
		t.Fatal("real route edit lost hold")
	}
}
