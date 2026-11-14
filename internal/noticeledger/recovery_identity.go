package noticeledger

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

var recoveryID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)

func validRecoveryRequest(q RecoveryRequest) bool {
	if !recoveryID.MatchString(q.RequestID) || !digestPattern.MatchString(q.TargetID) || !digestPattern.MatchString(q.ReviewFingerprint) || !digestPattern.MatchString(q.RoutingGeneration) {
		return false
	}
	switch q.Kind {
	case RetryBatch, ApproveRevision:
		return digestPattern.MatchString(q.ComponentFingerprint)
	case CloseBatch, CloseRevision:
		return q.ComponentFingerprint == ""
	}
	return false
}

func recoveryIntentDigest(d *recordData, q RecoveryRequest) string {
	return hashRecoveryTuple([]any{"oci-maintenance-router/recovery-intent", 1, d.EnvironmentID, d.NoticeID,
		q.RequestID, q.Kind, q.TargetID, q.ReviewFingerprint, q.RoutingGeneration, q.ComponentFingerprint})
}

func hashRecoveryTuple(tuple []any) string {
	data, _ := json.Marshal(tuple)
	return fmt.Sprintf("%x", sha256.Sum256(data))
}

func copyRecoveryRequest(q RecoveryRequest) RecoveryRequest {
	q.RequestID = strings.Clone(q.RequestID)
	q.Kind = ActionKind(strings.Clone(string(q.Kind)))
	q.TargetID = strings.Clone(q.TargetID)
	q.ReviewFingerprint = strings.Clone(q.ReviewFingerprint)
	q.RoutingGeneration = strings.Clone(q.RoutingGeneration)
	q.ComponentFingerprint = strings.Clone(q.ComponentFingerprint)
	return q
}

func recoveryRejection(reason string) bool {
	switch reason {
	case "stale_review", "target_missing", "target_terminal", "active_attempt",
		"source_unavailable", "source_changed", "configuration_changed", "component_changed",
		"ownership_changed", "secret_unavailable", "readiness_required", "no_eligible_work":
		return true
	}
	return false
}
