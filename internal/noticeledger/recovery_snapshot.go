package noticeledger

import "slices"

func copyRecovery(d *recoveryData) *recoveryData {
	if d == nil {
		return nil
	}
	next := &recoveryData{Actions: slices.Clone(d.Actions)}
	for i, action := range next.Actions {
		next.Actions[i] = copyRecoveryAction(action)
	}
	return next
}

func copyRecoveryAction(action RecoveryAction) RecoveryAction {
	action.BatchIDs = slices.Clone(action.BatchIDs)
	action.CompletedAt = copyText(action.CompletedAt)
	action.InvalidatedAt = copyText(action.InvalidatedAt)
	if action.Attempt != nil {
		attempt := *action.Attempt
		action.Attempt = &attempt
	}
	return action
}
