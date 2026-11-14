package noticeledger

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"oci-maintenance-router/internal/delivery"
)

func reviewHash(t *testing.T, r Record, kind ActionKind, target string) string {
	t.Helper()
	component := ""
	if kind == RetryBatch || kind == ApproveRevision {
		component = strings.Repeat("b", 64)
	}
	h, err := r.ReviewFingerprint(kind, target, r.Snapshot().Revisions[0].RoutingGeneration, component)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func TestRecoveryReviewMatchesIndependentExactTuple(t *testing.T) {
	const env = "c081fc9b5129be4e85c6039722858f2454d79fcd9219504f473d53dffd712378"
	r, err := Decode(recoveryGolden(t), env, noticeID)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile("testdata/review-fingerprint.json")
	if err != nil {
		t.Fatal(err)
	}
	var v struct {
		Kind                                  ActionKind
		Target, Generation, Component, SHA256 string
	}
	if err = json.Unmarshal(data, &v); err != nil {
		t.Fatal(err)
	}
	h, err := r.ReviewFingerprint(v.Kind, v.Target, v.Generation, v.Component)
	if err != nil || h != v.SHA256 {
		t.Fatal("review hash differs from independent wire tuple", h, err)
	}
}

func TestRecoveryReviewIgnoresRoutineObservationsAndRejectedRequests(t *testing.T) {
	r := recoveryRecord(t, observed(t, source(db)))
	id := r.Snapshot().Revisions[0].Digest
	want := reviewHash(t, r, CloseRevision, id)
	body := changedWire(t, r, func(w *wireRecord) {
		stamp := clock().Add(time.Hour).Format(timeFormat)
		w.UpdatedAt = stamp
		w.Revisions[0].FirstSeenAt = clock().Add(time.Second).Format(timeFormat)
		w.Revisions[0].LastSeenAt = stamp
		w.SourceOutcome.RecordedAt = stamp
	})
	env, notice := r.Identity()
	observedAgain, err := Decode(body, env, notice)
	if err != nil {
		t.Fatal(err)
	}
	if reviewHash(t, observedAgain, CloseRevision, id) != want {
		t.Fatal("observation clocks invalidate intent")
	}
	q := recoveryIntent(t)
	next, _, err := r.RejectRecovery(q, "readiness_required", clock().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if reviewHash(t, next, CloseRevision, id) != want {
		t.Fatal("rejected request changes target fingerprint")
	}
}

func TestRecoveryReviewBindsHoldsMembershipAndAttemptState(t *testing.T) {
	src := source(db)
	p := prepare(t, observed(t, src, db), preparations(t, src, db)...)
	r := recoveryRecord(t, p)
	id := r.Snapshot().Batches[0].Message.BatchID
	want := reviewHash(t, r, RetryBatch, id)
	s, _ := begin(t, r, id, clock())
	if reviewHash(t, s, RetryBatch, id) == want {
		t.Fatal("attempt state omitted")
	}
	held, err := r.Hold(delivery.ReasonOperator, clock())
	if err != nil {
		t.Fatal(err)
	}
	if reviewHash(t, held, RetryBatch, id) == want {
		t.Fatal("hold omitted")
	}
	changed := src
	changed.Reference = text("different revision")
	multiple, err := observer(t, nil).Observe(p, normalized(t, changed), false, lookups(db), clock())
	if err != nil {
		t.Fatal(err)
	}
	if reviewHash(t, recoveryRecord(t, multiple), RetryBatch, id) == want {
		t.Fatal("membership omitted")
	}
	values := preparations(t, src, db)
	values[0].SecretVersion++
	other := recoveryRecord(t, prepare(t, observed(t, src, db), values...))
	if reviewHash(t, other, RetryBatch, id) == want {
		t.Fatal("pinned secret version omitted")
	}
	generation := r.Snapshot().Revisions[0].RoutingGeneration
	for _, context := range [][2]string{{strings.Repeat("a", 64), strings.Repeat("b", 64)}, {generation, strings.Repeat("c", 64)}} {
		h, err := r.ReviewFingerprint(RetryBatch, id, context[0], context[1])
		if err != nil || h == want {
			t.Fatal("generation/component context omitted", err)
		}
	}
}

func TestRecoveryReviewRejectsInvalidContextAndUnknownTarget(t *testing.T) {
	r := recoveryRecord(t, observed(t, source(db)))
	id := r.Snapshot().Revisions[0].Digest
	gen := r.Snapshot().Revisions[0].RoutingGeneration
	for _, c := range []struct {
		kind                          ActionKind
		target, generation, component string
	}{
		{CloseRevision, id, "", ""}, {CloseRevision, id, gen, strings.Repeat("a", 64)},
		{ApproveRevision, id, gen, ""}, {"UNKNOWN", id, gen, ""},
		{CloseRevision, "", gen, ""}, {CloseRevision, strings.Repeat("f", 64), gen, ""},
		{CloseBatch, id, gen, ""},
	} {
		h, err := r.ReviewFingerprint(c.kind, c.target, c.generation, c.component)
		if err == nil || h != "" {
			t.Fatal("invalid review context usable")
		}
	}
	for _, old := range []Record{{}, observed(t, source(db))} {
		h, err := old.ReviewFingerprint(CloseRevision, id, gen, "")
		if err == nil || h != "" {
			t.Fatal("legacy/zero review context usable")
		}
	}
}
