package noticeledger

import (
	"bytes"
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
)

func recoveryIntent(t *testing.T) RecoveryRequest {
	t.Helper()
	b, err := os.ReadFile("testdata/recovery-intent.json")
	if err != nil {
		t.Fatal(err)
	}
	var vector struct {
		Request RecoveryRequest
		SHA256  string
	}
	if err := json.Unmarshal(b, &vector); err != nil {
		t.Fatal(err)
	}
	return vector.Request
}

func TestRecoveryRejectionRetainsIndependentIntentAndRepeat(t *testing.T) {
	r := recoveryRecord(t, observed(t, source(db)))
	req := recoveryIntent(t)
	before := encoded(t, r)
	next, action, err := r.RejectRecovery(req, "readiness_required", clock())
	if err != nil {
		t.Fatal(err)
	}
	if !next.Follows(r) || action.Status != RecoveryRejected || action.Reason != "readiness_required" || action.CompletedAt == nil || *action.CompletedAt != clock().Format(timeFormat) || action.Attempt != nil || action.BatchIDs == nil || len(action.BatchIDs) != 0 || action.Ambiguous {
		t.Fatal("invalid retained rejection")
	}
	body, _ := os.ReadFile("testdata/recovery-intent.json")
	var vector struct{ SHA256 string }
	if json.Unmarshal(body, &vector) != nil || vector.SHA256 != action.IntentDigest {
		t.Fatal("intent differs from independent tuple")
	}
	if !bytes.Equal(before, encoded(t, r)) {
		t.Fatal("history mutation changed predecessor")
	}
	reload(t, next)
	for _, at := range []time.Time{{}, clock().Add(-time.Hour), clock().Add(time.Hour)} {
		repeated, saved, err := next.RejectRecovery(req, "source_changed", at)
		if err != nil || repeated.Follows(next) || !bytes.Equal(encoded(t, repeated), encoded(t, next)) || !reflect.DeepEqual(saved, action) {
			t.Fatal("repeat consumed again or rechecked mutable prerequisites", err)
		}
	}
	changed := req
	changed.ReviewFingerprint = strings.Repeat("b", 64)
	bad, _, err := next.RejectRecovery(changed, "source_changed", clock())
	rejected(t, bad, err)
	if _, found, err := next.LookupRecovery(changed); err == nil || found {
		t.Fatal("changed intent matched")
	}
	found, ok, err := next.LookupRecovery(req)
	if err != nil || !ok || !reflect.DeepEqual(found, action) {
		t.Fatal("saved request missing", err)
	}
	req.RequestID = "missing"
	if _, found, err := next.LookupRecovery(req); err != nil || found {
		t.Fatal("missing request invented", err)
	}
}

func TestRecoveryHistoryOwnsReturnedResultsAndSortsRequests(t *testing.T) {
	r := recoveryRecord(t, observed(t, source(db)))
	req := recoveryIntent(t)
	req.RequestID = "z"
	next, action, err := r.RejectRecovery(req, "source_changed", clock())
	if err != nil {
		t.Fatal(err)
	}
	before := encoded(t, next)
	*action.CompletedAt = "changed"
	action.Request.RequestID = "changed"
	view := next.Snapshot()
	view.Recovery.Actions[0].Status = RecoveryApproved
	*view.Recovery.Actions[0].CompletedAt = "changed"
	loaded, ok, err := next.LookupRecovery(req)
	if err != nil || !ok {
		t.Fatal(err)
	}
	*loaded.CompletedAt = "changed"
	if !bytes.Equal(before, encoded(t, next)) {
		t.Fatal("returned history aliases saved state")
	}
	req.RequestID = "a"
	later, _, err := next.RejectRecovery(req, "source_unavailable", clock())
	if err != nil {
		t.Fatal(err)
	}
	actions := later.Snapshot().Recovery.Actions
	if len(actions) != 2 || actions[0].Request.RequestID != "a" || actions[1].Request.RequestID != "z" {
		t.Fatal("history not canonical")
	}
	if !bytes.Equal(before, encoded(t, next)) {
		t.Fatal("later request mutated history")
	}
	reload(t, later)
}

func TestRecoveryRequestRejectsMalformedIntentWithoutUsableProposal(t *testing.T) {
	r := recoveryRecord(t, observed(t, source(db)))
	base := recoveryIntent(t)
	for _, change := range []func(*RecoveryRequest){
		func(q *RecoveryRequest) { q.RequestID = "" },
		func(q *RecoveryRequest) { q.RequestID = strings.Repeat("x", 129) },
		func(q *RecoveryRequest) { q.RequestID = "bad id" },
		func(q *RecoveryRequest) { q.Kind = "OTHER" },
		func(q *RecoveryRequest) { q.TargetID = "" },
		func(q *RecoveryRequest) { q.ReviewFingerprint = strings.Repeat("A", 64) },
		func(q *RecoveryRequest) { q.RoutingGeneration = "" },
		func(q *RecoveryRequest) { q.ComponentFingerprint = strings.Repeat("b", 64) },
		func(q *RecoveryRequest) { q.Kind = RetryBatch },
	} {
		req := base
		change(&req)
		next, _, err := r.RejectRecovery(req, "stale_review", clock())
		rejected(t, next, err)
		if _, found, err := r.LookupRecovery(req); err == nil || found {
			t.Fatal("invalid lookup usable")
		}
	}
	for _, old := range []Record{{}, observed(t, source(db)), legacyRecord(t, observed(t, source(db)))} {
		next, _, err := old.RejectRecovery(base, "stale_review", clock())
		rejected(t, next, err)
	}
	for _, at := range []time.Time{{}, clock().Add(-time.Millisecond)} {
		next, _, err := r.RejectRecovery(base, "stale_review", at)
		rejected(t, next, err)
	}
	next, _, err := r.RejectRecovery(base, "raw provider detail", clock())
	rejected(t, next, err)
}
