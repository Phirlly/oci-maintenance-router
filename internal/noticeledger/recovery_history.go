package noticeledger

import (
	"slices"
	"strings"
	"time"
)

// LookupRecovery compares immutable intent before any current-state prerequisites.
func (r Record) LookupRecovery(req RecoveryRequest) (RecoveryAction, bool, error) {
	i, _, err := r.recoveryLookup(req)
	if err != nil || i < 0 {
		return RecoveryAction{}, false, err
	}
	return copyRecoveryAction(r.data.Recovery.Actions[i]), true, nil
}

// RejectRecovery proposes a controlled prerequisite rejection, never persistence.
func (r Record) RejectRecovery(req RecoveryRequest, reason string, at time.Time) (Record, RecoveryAction, error) {
	i, digest, err := r.recoveryLookup(req)
	if err != nil {
		return Record{}, RecoveryAction{}, err
	}
	if i >= 0 {
		return r, copyRecoveryAction(r.data.Recovery.Actions[i]), nil
	}
	if !recoveryRejection(reason) {
		return Record{}, RecoveryAction{}, &Error{"invalid_rejection"}
	}
	stamp, err := r.operationTime(at)
	if err != nil {
		return Record{}, RecoveryAction{}, err
	}
	action := RecoveryAction{Request: copyRecoveryRequest(req), IntentDigest: digest, RecordedAt: stamp,
		Status: RecoveryRejected, Reason: strings.Clone(reason), CompletedAt: &stamp, BatchIDs: []string{}}
	d := r.data.copy()
	d.UpdatedAt = stamp
	d.Recovery.Actions = append(d.Recovery.Actions, action)
	sortRecovery(d.Recovery)
	next, err := r.finalize(d, at)
	if err != nil {
		return Record{}, RecoveryAction{}, err
	}
	return next, copyRecoveryAction(action), nil
}

func (r Record) recoveryLookup(req RecoveryRequest) (int, string, error) {
	if r.data == nil || r.data.SchemaVersion != 3 || r.data.Recovery == nil {
		return -1, "", &Error{"recovery_migration_required"}
	}
	if !validRecoveryRequest(req) {
		return -1, "", &Error{"invalid_recovery_request"}
	}
	digest := recoveryIntentDigest(r.data, req)
	for i, action := range r.data.Recovery.Actions {
		if action.Request.RequestID == req.RequestID {
			if action.IntentDigest != digest {
				return -1, "", &Error{"recovery_intent_conflict"}
			}
			return i, digest, nil
		}
	}
	return -1, digest, nil
}

func sortRecovery(d *recoveryData) {
	slices.SortFunc(d.Actions, func(a, b RecoveryAction) int { return strings.Compare(a.Request.RequestID, b.Request.RequestID) })
}
