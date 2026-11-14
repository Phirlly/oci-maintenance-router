package noticeledger

import (
	"reflect"
	"testing"
	"time"

	"oci-maintenance-router/internal/delivery"
)

func preparationConfig(v map[string]any) {
	v["schemaVersion"] = 2
	v["mode"] = "PREVIEW"
	v["installationId"] = "installation-1"
	v["preparation"] = map[string]any{"initialize": true}
}

func TestPreparationSchemaUpgradePreservesPendingWorkAndReceipts(t *testing.T) {
	for _, state := range []delivery.State{delivery.Prepared, delivery.RetryWait, delivery.APIAccepted} {
		src := source(db)
		r := prepare(t, observed(t, src, db), preparations(t, src, db)...)
		if state != delivery.Prepared {
			var ref delivery.AttemptRef
			r, ref = begin(t, r, r.Snapshot().Batches[0].Message.BatchID, clock())
			status := 503
			retryAt := clock().Add(time.Minute)
			if state == delivery.APIAccepted {
				status = 202
				retryAt = time.Time{}
			}
			var err error
			r, err = r.Finish(ref, status, clock(), retryAt)
			if err != nil {
				t.Fatal(err)
			}
		}
		before := r.Snapshot()
		o := observer(t, preparationConfig)
		next, err := o.Observe(r, normalized(t, src), false, lookups(db), clock())
		if err != nil {
			t.Fatal(err)
		}
		after := next.Snapshot()
		if after.HoldReason != "" || after.Revisions[0].RoutingGeneration != before.Revisions[0].RoutingGeneration || !reflect.DeepEqual(after.Batches, before.Batches) {
			t.Fatal("schema-only upgrade changed immutable delivery or held work")
		}
		body, err := next.Encode()
		if err != nil {
			t.Fatal(err)
		}
		restored, err := Decode(body, after.EnvironmentID, after.NoticeID)
		if err != nil || !reflect.DeepEqual(restored.Snapshot().Batches, before.Batches) {
			t.Fatal("restart lost saved work", err)
		}
	}
}

func TestPreparationSchemaRealRoutingChangeStillHoldsPendingWork(t *testing.T) {
	src := source(db)
	r := prepare(t, observed(t, src, db), preparations(t, src, db)...)
	o := observer(t, func(v map[string]any) { preparationConfig(v); v["pagerDutyRegion"] = "EU" })
	next, err := o.Observe(r, normalized(t, src), false, lookups(db), clock())
	if err != nil {
		t.Fatal(err)
	}
	b := next.Snapshot().Batches[0]
	if b.State != delivery.ReviewRequired || b.Reason != delivery.ReasonConfigurationChanged {
		t.Fatal("real routing change lost hold")
	}
}
