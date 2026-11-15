package deployment

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"oci-maintenance-router/internal/delivery"
	"oci-maintenance-router/internal/noticeledger"
)

func TestClosureReviewShowsAllBatchStatesAndOnlySelectedResources(t *testing.T) {
	for _, state := range []string{"PREPARED", "SENDING", "RETRY_WAIT", "API_ACCEPTED", "REVIEW_REQUIRED", "CLOSED"} {
		t.Run(state, func(t *testing.T) {
			f := preparedClosureFixture(t)
			batch := f.record.Snapshot().Batches[0]
			id := batch.Message.BatchID
			if state != "PREPARED" && state != "CLOSED" {
				next, ref, err := f.record.Begin(id, delivery.AttemptContext{OwnerToken: strings.Repeat("a", 64), RoutingGeneration: batch.Message.RoutingGeneration, Now: reviewTime(), LeaseExpiresAt: reviewTime().Add(time.Minute)})
				if err != nil {
					t.Fatal(err)
				}
				f.save(t, next)
				if state != "SENDING" {
					status := 503
					if state == "API_ACCEPTED" {
						status = 202
					}
					if state == "REVIEW_REQUIRED" {
						status = 400
					}
					retryAt := time.Time{}
					if status == 503 {
						retryAt = reviewTime().Add(2 * time.Second)
					}
					next, err = f.record.Finish(ref, status, reviewTime().Add(time.Second), retryAt)
					if err != nil {
						t.Fatal(err)
					}
					f.save(t, next)
				}
			}
			if state == "CLOSED" {
				q := noticeledger.RecoveryRequest{RequestID: "closed-before", Kind: noticeledger.CloseBatch, TargetID: id, RoutingGeneration: batch.Message.RoutingGeneration}
				q.ReviewFingerprint, _ = f.record.ReviewFingerprint(q.Kind, q.TargetID, q.RoutingGeneration, "")
				next, _, err := f.record.CloseRecovery(q, reviewTime())
				if err != nil {
					t.Fatal(err)
				}
				f.save(t, next)
			}
			before := businessObjects(f.objects)
			rows := closurePages(t, f, "CLOSE_BATCH", id)
			resources, batches, codes := 0, 0, 0
			for _, row := range rows {
				switch row["kind"] {
				case "RESOURCE":
					resources++
					if row["batchId"] != id {
						t.Fatal("other batch resource disclosed as selected")
					}
				case "BATCH":
					batches++
					if row["batchId"] != id || row["state"] != state || row["routingGeneration"] != batch.Message.RoutingGeneration {
						t.Fatal(row)
					}
				case "CLOSURE":
					codes++
				}
			}
			want := 1
			if state == "SENDING" || state == "API_ACCEPTED" || state == "CLOSED" {
				want = 0
			}
			if resources != 1 || batches != 1 || codes != want || !reflect.DeepEqual(before, businessObjects(f.objects)) {
				t.Fatal("wrong scoped review", resources, batches, codes)
			}
			all := closurePages(t, f, "CLOSE_REVISION", f.notice.Key().Digest)
			peers := 0
			for _, row := range all {
				if row["kind"] == "BATCH" {
					peers++
				}
			}
			if peers != 2 {
				t.Fatal("revision hid preserved peer")
			}
		})
	}
}
