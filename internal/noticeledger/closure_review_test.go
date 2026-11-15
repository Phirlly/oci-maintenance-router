package noticeledger

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestClosureEligibilityUsesRetainedStateWithoutExpiryOrMutation(t *testing.T) {
	r, _, ref, at := claimedRetry(t, true)
	before := encoded(t, r)
	for _, kind := range []ActionKind{CloseBatch, CloseRevision} {
		target := ref.BatchID
		if kind == CloseRevision {
			target = r.Snapshot().Revisions[0].Digest
		}
		if blocker, err := r.ClosureBlocker(kind, target); err != nil || blocker != "active_attempt" {
			t.Fatal(blocker, err)
		}
	}
	if !bytes.Equal(before, encoded(t, r)) {
		t.Fatal("eligibility altered sending history")
	}
	r, err := r.StopUnsent(ref, at)
	if err != nil {
		t.Fatal(err)
	}
	if blocker, err := r.ClosureBlocker(CloseBatch, ref.BatchID); err != nil || blocker != "" {
		t.Fatal(blocker, err)
	}
	closed, _, err := r.CloseRecovery(closeRequest(t, r, CloseBatch, ref.BatchID), at.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if blocker, err := closed.ClosureBlocker(CloseBatch, ref.BatchID); err != nil || blocker != "target_terminal" {
		t.Fatal(blocker, err)
	}
	if blocker, err := closed.ClosureBlocker(CloseRevision, strings.Repeat("f", 64)); err != nil || blocker != "target_missing" {
		t.Fatal(blocker, err)
	}
}

func TestClosureEligibilityRequiresMigratedExactCloseTarget(t *testing.T) {
	r := recoveryRecord(t, observed(t, source(db)))
	for _, q := range []struct {
		r      Record
		kind   ActionKind
		target string
	}{
		{Record{}, CloseRevision, strings.Repeat("a", 64)},
		{observed(t, source(db)), CloseRevision, r.Snapshot().Revisions[0].Digest},
		{r, RetryBatch, strings.Repeat("a", 64)}, {r, CloseBatch, "bad"},
	} {
		if _, err := q.r.ClosureBlocker(q.kind, q.target); err == nil {
			t.Fatal("invalid closure query admitted")
		}
	}
}
