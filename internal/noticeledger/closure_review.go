package noticeledger

// ClosureBlocker describes retained eligibility only. It neither admits an action
// nor predicts a future conditional write. SENDING remains blocked after expiry.
func (r Record) ClosureBlocker(kind ActionKind, target string) (string, error) {
	if r.data == nil || !recoveryVersion(r.data.SchemaVersion) {
		return "", &Error{"recovery_migration_required"}
	}
	if (kind != CloseBatch && kind != CloseRevision) || !digestPattern.MatchString(target) {
		return "", &Error{"invalid_recovery_request"}
	}
	_, reason := closeTargets(r.data, RecoveryRequest{Kind: kind, TargetID: target})
	return reason, nil
}
