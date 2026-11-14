package noticeledger

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"oci-maintenance-router/internal/configuration"
	"oci-maintenance-router/internal/delivery"
)

func recoveryConfig(v map[string]any) {
	liveConfiguration(v)
	v["schemaVersion"] = 7
	v["recovery"] = map[string]any{"requestId": "recovery-1", "noticeId": noticeID, "kind": "RETRY_BATCH",
		"targetId": strings.Repeat("a", 64), "reviewFingerprint": strings.Repeat("b", 64),
		"routingGeneration": strings.Repeat("c", 64), "componentFingerprint": strings.Repeat("d", 64)}
}

func TestRecoveryConfigurationPreservesSavedMaintenanceWork(t *testing.T) {
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
		for _, mode := range []string{"PREVIEW", "PAUSED", "TEST", "LIVE"} {
			next, err := observer(t, func(v map[string]any) { recoveryConfig(v); v["mode"] = mode }).Observe(r, normalized(t, src), false, lookups(db), clock())
			if err != nil {
				t.Fatal(err)
			}
			after := reload(t, next).Snapshot()
			if after.SchemaVersion != before.SchemaVersion || after.Recovery != nil || after.HoldReason != "" || after.Revisions[0].RoutingGeneration != before.Revisions[0].RoutingGeneration || !reflect.DeepEqual(before.Batches, after.Batches) {
				t.Fatal("configuration executed recovery or altered saved work")
			}
		}
	}
}

func TestRecoveryConfigurationPreservesRetainedManualOutcome(t *testing.T) {
	r, _, ref, at := claimedRetry(t, true)
	r, err := r.Finish(ref, 202, at, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	before := r.Snapshot()
	next, err := observer(t, recoveryConfig).Observe(r, normalized(t, source(db)), false, lookups(db), at.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	after := reload(t, next).Snapshot()
	if !reflect.DeepEqual(before.Recovery, after.Recovery) || !reflect.DeepEqual(before.Batches, after.Batches) {
		t.Fatal("configuration rewrote manual history")
	}
}

func TestRecoveryConfigurationStillHoldsChangedRouting(t *testing.T) {
	src := source(db)
	r := prepare(t, observed(t, src, db), preparations(t, src, db)...)
	next, err := observer(t, func(v map[string]any) { recoveryConfig(v); v["pagerDutyRegion"] = "EU" }).Observe(r, normalized(t, src), false, lookups(db), clock())
	if err != nil {
		t.Fatal(err)
	}
	b := next.Snapshot().Batches[0]
	if b.State != delivery.ReviewRequired || b.Reason != delivery.ReasonConfigurationChanged || next.Snapshot().Recovery != nil {
		t.Fatal("recovery intent bypassed routing hold")
	}
}

func TestDecodedRecoveryRequestsMatchNoticeOwnedContract(t *testing.T) {
	r := recoveryRecord(t, observed(t, source(db)))
	for _, kind := range []string{"RETRY_BATCH", "APPROVE_REVISION", "CLOSE_BATCH", "CLOSE_REVISION"} {
		wire := settings(func(v map[string]any) {
			recoveryConfig(v)
			q := v["recovery"].(map[string]any)
			q["kind"] = kind
			if strings.HasPrefix(kind, "CLOSE_") {
				delete(q, "componentFingerprint")
			}
		})
		c, err := configuration.Decode(wire, catalog())
		if err != nil {
			t.Fatal(err)
		}
		q := c.Recovery
		request := RecoveryRequest{RequestID: q.RequestID, Kind: ActionKind(q.Kind), TargetID: q.TargetID, ReviewFingerprint: q.ReviewFingerprint, RoutingGeneration: q.RoutingGeneration, ComponentFingerprint: q.ComponentFingerprint}
		if q.NoticeID != r.Snapshot().NoticeID {
			t.Fatal("wrong notice identity")
		}
		if _, found, err := r.LookupRecovery(request); err != nil || found {
			t.Fatal("decoded request does not match model or invented action", err)
		}
	}
}
