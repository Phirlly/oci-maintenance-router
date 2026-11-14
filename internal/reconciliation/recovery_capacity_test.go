package reconciliation

import (
	"bytes"
	"encoding/json"
	"oci-maintenance-router/internal/delivery"
	"oci-maintenance-router/internal/noticeledger"
	"strings"
	"testing"
	"time"
)

// Fill retained display text through the real canonical decoder. This synthetic
// fixture leaves immutable delivery bytes untouched and uses no provider data.
func fullRecoveryRecord(t *testing.T, r noticeledger.Record) noticeledger.Record {
	t.Helper()
	body, _ := r.Encode()
	reference, _ := json.Marshal(r.Snapshot().Revisions[0].Source.Reference)
	field := append([]byte(`"reference":`), reference...)
	if !bytes.Contains(body, field) {
		t.Fatal("capacity reference missing")
	}
	env, id := r.Identity()
	low, high, best := 0, 1<<20, r
	for low <= high {
		middle := (low + high) / 2
		text, _ := json.Marshal(strings.Repeat("x", middle))
		input := bytes.Replace(body, field, append([]byte(`"reference":`), text...), 1)
		candidate, err := noticeledger.Decode(input, env, id)
		if err == nil {
			best, low = candidate, middle+1
		} else {
			if e, ok := err.(*noticeledger.Error); !ok || e.Code != "too_large" {
				t.Fatal("invalid capacity fixture", err)
			}
			high = middle - 1
		}
	}
	if low < 100000 {
		t.Fatal("capacity boundary not reached")
	}
	return best
}

func TestRecoveryCapacityFailurePreservesRecordAndDoesNotConsumeRequest(t *testing.T) {
	for _, stage := range []string{"migration", "closure", "rejection"} {
		n := processingNotice(t, 1, processingDB)
		f := processingFixtureFor(t, n)
		old := f.seedBatch(t, n, time.Now().Add(-time.Minute), delivery.Prepared, true)
		q := reviewedRecovery(t, old, noticeledger.CloseBatch, old.Snapshot().Batches[0].Message.BatchID)
		if stage != "migration" {
			var err error
			old, err = old.MigrateRecovery(time.Now())
			if err != nil {
				t.Fatal(err)
			}
		}
		if stage == "rejection" {
			q["routingGeneration"] = strings.Repeat("f", 64)
		}
		full := fullRecoveryRecord(t, old)
		before, _ := full.Encode()
		f.live.objects.seed(noticeName(n.Key().NoticeID), before)
		p := recoveryOnlyProcessor(t, f, "PAUSED", q)
		writes := len(f.live.objects.noticeWrites())
		r, err := p.Run(leaseBudget(t), f.live.lease)
		after, _ := f.saved(t, n.Key().NoticeID).Encode()
		if err == nil || r.Snapshot().Completed || r.Snapshot().Recovery != nil || f.sent.Load() != 0 || len(f.live.objects.noticeWrites()) != writes || !bytes.Equal(before, after) {
			t.Fatal("capacity failure changed saved work", stage, err)
		}
	}
}
