package noticeledger

import "slices"

func recoveryApproval(d *recordData, id string) (RecoveryAction, bool) {
	if d.Recovery != nil {
		for _, a := range d.Recovery.Actions {
			if a.Status == RecoveryApproved && slices.Contains(a.BatchIDs, id) {
				return a, true
			}
		}
	}
	return RecoveryAction{}, false
}

func activeApproval(d *recordData, a RecoveryAction) bool {
	i := revisionIndex(d, a.Request.TargetID)
	return a.Status == RecoveryApproved && a.InvalidatedAt == nil && i >= 0 &&
		d.Revisions[i].RoutingGeneration == a.Request.RoutingGeneration &&
		d.SourceOutcome.State == SourceAvailable && d.SourceOutcome.RevisionDigest == a.Request.TargetID &&
		!recoveryRevisionClosed(d, a.Request.TargetID)
}

func invalidateApprovals(d *recordData, reason string) {
	if d.Recovery == nil {
		return
	}
	for i, a := range d.Recovery.Actions {
		if a.Status == RecoveryApproved && a.InvalidatedAt == nil {
			at := d.UpdatedAt
			a.InvalidatedAt = &at
			a.InvalidationReason = reason
			d.Recovery.Actions[i] = a
		}
	}
}

func syncApprovalInvalidation(previous *recordData, d *recordData) {
	if !recoveryVersion(d.SchemaVersion) {
		return
	}
	changed := ""
	if previous != nil {
		if len(previous.Revisions) != len(d.Revisions) {
			changed = "source_changed"
		} else {
			for i, v := range d.Revisions {
				old := previous.Revisions[i]
				if old.Digest != v.Digest {
					changed = "source_changed"
					break
				}
				if old.RoutingGeneration != v.RoutingGeneration {
					changed = "configuration_changed"
					break
				}
			}
		}
	}
	for i, a := range d.Recovery.Actions {
		if a.Status != RecoveryApproved || a.InvalidatedAt != nil {
			continue
		}
		reason := changed
		if d.SourceOutcome.State != SourceAvailable {
			reason = "source_unavailable"
		} else if d.SourceOutcome.RevisionDigest != a.Request.TargetID {
			reason = "source_changed"
		}
		v := revisionIndex(d, a.Request.TargetID)
		if v >= 0 && d.Revisions[v].RoutingGeneration != a.Request.RoutingGeneration {
			reason = "configuration_changed"
		}
		if reason != "" {
			at := d.UpdatedAt
			a.InvalidatedAt = &at
			a.InvalidationReason = reason
			d.Recovery.Actions[i] = a
		}
	}
}
