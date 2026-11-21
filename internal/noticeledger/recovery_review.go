package noticeledger

import "strings"

// ReviewFingerprint binds retained target state and supplied configuration/component
// context. It cannot verify that those contexts are current or authorize an action.
func (r Record) ReviewFingerprint(kind ActionKind, target, generation, component string) (string, error) {
	if r.data == nil || !recoveryVersion(r.data.SchemaVersion) {
		return "", &Error{"recovery_migration_required"}
	}
	q := RecoveryRequest{RequestID: "review", Kind: kind, TargetID: target, RoutingGeneration: generation,
		ComponentFingerprint: component, ReviewFingerprint: strings.Repeat("0", 64)}
	if !validRecoveryRequest(q) {
		return "", &Error{"invalid_recovery_request"}
	}
	projection, err := recoveryTargetView(r.data, kind, target)
	if err != nil {
		return "", err
	}
	members := make([]any, 0, len(r.data.Revisions))
	for _, v := range r.data.Revisions {
		chain := ""
		if v.Source.ChainID != nil {
			chain = *v.Source.ChainID
		}
		members = append(members, []any{v.Digest, chain, v.RoutingGeneration, v.Assessment, v.AssessmentReason})
	}
	o := r.data.SourceOutcome
	return hashRecoveryTuple([]any{"oci-maintenance-router/recovery-review", 1, r.data.EnvironmentID, r.data.NoticeID,
		kind, target, generation, component, r.data.HoldReason, []any{o.State, o.RevisionDigest, o.HTTPStatus}, members, projection}), nil
}

func recoveryTargetView(d *recordData, kind ActionKind, target string) ([]any, error) {
	ids := map[string]bool{}
	if kind == RetryBatch || kind == CloseBatch {
		b, ok := d.delivery.Batch(target)
		if !ok {
			return nil, &Error{"recovery_target_missing"}
		}
		ids[target] = true
		return []any{recoveryBatchView(b), recoveryRevisionClosed(d, b.Message.Revision.Digest), recoveryApprovalView(d, ids)}, nil
	}
	i := revisionIndex(d, target)
	if i < 0 {
		return nil, &Error{"recovery_target_missing"}
	}
	batches := []any{}
	for _, b := range d.delivery.Batches() {
		if b.Message.Revision.Digest == target {
			ids[b.Message.BatchID] = true
			batches = append(batches, recoveryBatchView(b))
		}
	}
	return []any{d.Revisions[i].Resources, batches, recoveryRevisionClosed(d, target), recoveryApprovalView(d, ids)}, nil
}

func recoveryRevisionClosed(d *recordData, digest string) bool {
	if d.Recovery == nil {
		return false
	}
	for _, a := range d.Recovery.Actions {
		if a.Status == RecoveryClosed && a.Request.Kind == CloseRevision && a.Request.TargetID == digest {
			return true
		}
	}
	return false
}

func recoveryApprovalView(d *recordData, ids map[string]bool) []any {
	result := []any{}
	for _, a := range d.Recovery.Actions {
		if a.Status != RecoveryApproved {
			continue
		}
		for _, id := range a.BatchIDs {
			if ids[id] {
				at := ""
				if a.InvalidatedAt != nil {
					at = *a.InvalidatedAt
				}
				result = append(result, []any{a.Request.RequestID, at, a.InvalidationReason})
				break
			}
		}
	}
	return result
}
