package noticeledger

import (
	"reflect"
	"testing"
	"time"

	"oci-maintenance-router/internal/delivery"
)

func operatorConfig(v map[string]any) {
	diagnosticConfig(v)
	v["schemaVersion"] = 5
	v["operatorTest"] = map[string]any{"requestId": "operator-1", "subscriptionId": "ocid1.onssubscription.oc1.iad.operator", "endpoint": "Team@example.invalid"}
}

func TestOperatorConfigurationPreservesSavedWorkAcrossRestart(t *testing.T) {
	for _, state := range []delivery.State{delivery.Prepared, delivery.RetryWait, delivery.APIAccepted} {
		src := source(db)
		r := prepare(t, observed(t, src, db), preparations(t, src, db)...)
		if state != delivery.Prepared {
			var ref delivery.AttemptRef
			r, ref = begin(t, r, r.Snapshot().Batches[0].Message.BatchID, clock())
			status, retry := 503, clock().Add(time.Minute)
			if state == delivery.APIAccepted {
				status, retry = 202, time.Time{}
			}
			var err error
			r, err = r.Finish(ref, status, clock(), retry)
			if err != nil {
				t.Fatal(err)
			}
		}
		before := r.Snapshot()
		for _, change := range []func(map[string]any){operatorConfig, func(v map[string]any) {
			operatorConfig(v)
			v["operatorTest"] = map[string]any{"requestId": "next", "subscriptionId": "ocid1.onssubscription.oc1.iad.other", "endpoint": "Other@example.invalid", "receiptCode": "OMR-OP-0123456789abcdef0123456789abcdef"}
		}} {
			next, err := observer(t, change).Observe(r, normalized(t, src), false, lookups(db), clock())
			if err != nil {
				t.Fatal(err)
			}
			after := next.Snapshot()
			if after.HoldReason != "" || after.Revisions[0].RoutingGeneration != before.Revisions[0].RoutingGeneration || !reflect.DeepEqual(after.Batches, before.Batches) {
				t.Fatal("operator controls changed saved maintenance work")
			}
			wire, err := next.Encode()
			if err != nil {
				t.Fatal(err)
			}
			restored, err := Decode(wire, after.EnvironmentID, after.NoticeID)
			if err != nil || !reflect.DeepEqual(restored.Snapshot().Batches, before.Batches) {
				t.Fatal("restart lost original payloads, pins or receipts", err)
			}
		}
	}
}

func TestOperatorConfigurationStillHoldsRealRoutingChanges(t *testing.T) {
	src := source(db)
	r := prepare(t, observed(t, src, db), preparations(t, src, db)...)
	next, err := observer(t, func(v map[string]any) { operatorConfig(v); v["pagerDutyRegion"] = "EU" }).Observe(r, normalized(t, src), false, lookups(db), clock())
	if err != nil {
		t.Fatal(err)
	}
	b := next.Snapshot().Batches[0]
	if b.State != delivery.ReviewRequired || b.Reason != delivery.ReasonConfigurationChanged {
		t.Fatal("real route edit no longer holds pending work")
	}
}
