package noticeledger

import (
	"strings"
	"testing"
	"time"
)

func rejectRecoveryWire(t *testing.T, r Record, changes map[string]func(*wireRecord)) {
	t.Helper()
	env, id := r.Identity()
	for name, change := range changes {
		t.Run(name, func(t *testing.T) { bad, err := Decode(changedWire(t, r, change), env, id); rejected(t, bad, err) })
	}
}

func TestRecoveryCodecRejectsInvalidRetryState(t *testing.T) {
	r, _, ref, at := claimedRetry(t, false)
	rejectRecoveryWire(t, r, map[string]func(*wireRecord){
		"missing_action":   func(w *wireRecord) { w.Recovery.Actions = []RecoveryAction{} },
		"duplicate_action": func(w *wireRecord) { w.Recovery.Actions = append(w.Recovery.Actions, w.Recovery.Actions[0]) },
		"intent":           func(w *wireRecord) { w.Recovery.Actions[0].IntentDigest = strings.Repeat("a", 64) },
		"missing_attempt":  func(w *wireRecord) { w.Recovery.Actions[0].Attempt = nil },
		"attempt_target":   func(w *wireRecord) { w.Recovery.Actions[0].Attempt.BatchID = strings.Repeat("a", 64) },
		"owner":            func(w *wireRecord) { w.Recovery.Actions[0].Attempt.OwnerToken = "wrong" },
		"count":            func(w *wireRecord) { w.Recovery.Actions[0].Attempt.AttemptCount++ },
		"zero_count":       func(w *wireRecord) { w.Recovery.Actions[0].Attempt.AttemptCount = 0 },
		"invalid_expiry":   func(w *wireRecord) { w.Recovery.Actions[0].Attempt.ExpiresAt = "bad" },
		"long_expiry": func(w *wireRecord) {
			w.Recovery.Actions[0].Attempt.ExpiresAt = at.Add(601 * time.Second).Format(timeFormat)
		},
		"future_start":      func(w *wireRecord) { w.Recovery.Actions[0].RecordedAt = at.Add(time.Second).Format(timeFormat) },
		"completion":        func(w *wireRecord) { w.Recovery.Actions[0].CompletedAt = text(at.Format(timeFormat)) },
		"reason":            func(w *wireRecord) { w.Recovery.Actions[0].Reason = "transient" },
		"ambiguity":         func(w *wireRecord) { w.Recovery.Actions[0].Ambiguous = true },
		"batch_association": func(w *wireRecord) { w.Recovery.Actions[0].BatchIDs = []string{ref.BatchID} },
		"null_batch_ids":    func(w *wireRecord) { w.Recovery.Actions[0].BatchIDs = nil },
		"invalidation":      func(w *wireRecord) { w.Recovery.Actions[0].InvalidatedAt = text(at.Format(timeFormat)) },
		"status":            func(w *wireRecord) { w.Recovery.Actions[0].Status = "UNKNOWN" },
	})
	done, err := r.Finish(ref, 503, at, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	rejectRecoveryWire(t, done, map[string]func(*wireRecord){
		"missing_completion": func(w *wireRecord) { w.Recovery.Actions[0].CompletedAt = nil },
		"future_completion":  func(w *wireRecord) { w.Recovery.Actions[0].CompletedAt = text(at.Add(time.Second).Format(timeFormat)) },
		"earlier_completion": func(w *wireRecord) { w.Recovery.Actions[0].CompletedAt = text(at.Add(-time.Second).Format(timeFormat)) },
		"invented_expiry":    func(w *wireRecord) { w.Recovery.Actions[0].Reason = "expired"; w.Recovery.Actions[0].Ambiguous = true },
		"wrong_result":       func(w *wireRecord) { w.Recovery.Actions[0].Reason = "permanent" },
		"invented_acceptance": func(w *wireRecord) {
			w.Recovery.Actions[0].Status = RecoveryAccepted
			w.Recovery.Actions[0].Reason = ""
		},
	})
}

func TestRecoveryCodecRejectsInvalidApprovalState(t *testing.T) {
	r, _, a := approvedRecord(t, true)
	rejectRecoveryWire(t, r, map[string]func(*wireRecord){
		"missing_source": func(w *wireRecord) { w.SourceOutcome = nil },
		"no_members":     func(w *wireRecord) { w.Recovery.Actions[0].BatchIDs = []string{} },
		"unknown_member": func(w *wireRecord) { w.Recovery.Actions[0].BatchIDs = []string{strings.Repeat("a", 64)} },
		"duplicate_member": func(w *wireRecord) {
			w.Recovery.Actions[0].BatchIDs = append(w.Recovery.Actions[0].BatchIDs, a.BatchIDs[0])
		},
		"reason":                      func(w *wireRecord) { w.Recovery.Actions[0].Reason = "operator" },
		"ambiguity":                   func(w *wireRecord) { w.Recovery.Actions[0].Ambiguous = true },
		"missing_completion":          func(w *wireRecord) { w.Recovery.Actions[0].CompletedAt = nil },
		"invalidation_reason_only":    func(w *wireRecord) { w.Recovery.Actions[0].InvalidationReason = "operator" },
		"invalidation_missing_reason": func(w *wireRecord) { w.Recovery.Actions[0].InvalidatedAt = text(clock().Format(timeFormat)) },
		"future_invalidation": func(w *wireRecord) {
			w.Recovery.Actions[0].InvalidatedAt = text(clock().Add(time.Second).Format(timeFormat))
			w.Recovery.Actions[0].InvalidationReason = "operator"
		},
		"inactive_source":    func(w *wireRecord) { w.SourceOutcome.State = SourceUnknown; w.SourceOutcome.RevisionDigest = "" },
		"changed_generation": func(w *wireRecord) { w.Revisions[0].RoutingGeneration = strings.Repeat("a", 64) },
	})
}
