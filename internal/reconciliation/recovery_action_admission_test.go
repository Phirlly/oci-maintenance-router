package reconciliation

import (
	"bytes"
	"context"
	"errors"
	"oci-maintenance-router/internal/delivery"
	"oci-maintenance-router/internal/noticeledger"
	"oci-maintenance-router/internal/routing"
	"strings"
	"testing"
	"time"
)

// Exercise the action's final admission using genuine stored handles, current
// controls, lease and baseline. Full Run tests separately prove source review.
func TestRecoveryActionAdmissionPreservesUnconsumedWorkOnFailure(t *testing.T) {
	for _, approval := range []bool{false, true} {
		for _, fault := range []string{"pacing", "record-capacity", "write-authority", "result-capacity", "baseline-changed"} {
			if approval && fault == "result-capacity" {
				continue
			}
			n := processingNotice(t, 1, processingDB)
			f := processingFixtureFor(t, n)
			old := seedRecoveryRevision(t, f, n)
			kind, target := noticeledger.ApproveRevision, n.Key().Digest
			if !approval {
				old = f.seedBatch(t, n, time.Now().Add(-time.Minute), delivery.Prepared, true)
				kind, target = noticeledger.RetryBatch, old.Snapshot().Batches[0].Message.BatchID
			}
			var err error
			old, err = old.MigrateRecovery(time.Now())
			if err != nil {
				t.Fatal(err)
			}
			if fault == "record-capacity" {
				old = fullRecoveryRecord(t, old)
			}
			q := reviewedRecovery(t, old, kind, target)
			body, _ := old.Encode()
			f.live.objects.seed(noticeName(n.Key().NoticeID), body)
			f.configure(t, func(v map[string]any) { v["schemaVersion"], v["recovery"] = 7, q })
			ctx := leaseBudget(t)
			r := processingAttemptRun(t, f, ctx, strings.Repeat("d", 64))
			saved, err := r.store.Load(ctx, n.Key().NoticeID)
			if err != nil {
				t.Fatal(err)
			}
			m, err := f.p.mysql["us-ashburn-1"](ctx, processingDB)
			if err != nil {
				t.Fatal(err)
			}
			lookups := map[string]routing.Lookup{processingDB: {State: routing.Found, Metadata: m}}
			switch fault {
			case "pacing":
				r.started = true
				f.p.wait = func(context.Context, time.Duration) error { return errors.New("wait failed") }
			case "write-authority":
				f.live.objects.seed("configuration/router.json", []byte("changed"))
			case "result-capacity":
				r.result.snapshot.Attempts = make([]ProcessingAttempt, 1000)
			case "baseline-changed":
				// Both baselines are genuine; the one selected now differs from
				// the one retained by this action's prior complete pass.
				r.baseline = liveSelectorFixtureFor(t, n).baseline
			}
			writes := len(f.live.objects.noticeWrites())
			if approval {
				err = r.approveRecovery(saved, n, lookups)
			} else {
				err = r.retryRecovery(saved, n, lookups)
			}
			after, _ := f.saved(t, n.Key().NoticeID).Encode()
			if err == nil || f.sent.Load() != 0 || r.result.snapshot.Recovery != nil || r.result.snapshot.Prepared != 0 || len(f.live.objects.noticeWrites()) != writes || !bytes.Equal(body, after) {
				t.Fatal("failed admission consumed or changed work", approval, fault, err)
			}
		}
	}
}
