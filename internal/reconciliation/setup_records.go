package reconciliation

import "context"

// These reads validate selection/intent only. Callers must separately establish
// acceptance and either supplied-code attestation or saved confirmation.
func (c setupControls) diagnosticRecord(ctx context.Context, objects DiagnosticObjects, head setupCurrent, release string) (DiagnosticRecord, DiagnosticIntent, error) {
	s, _ := NewDiagnosticStore(objects, c.environment, c.config.InstallationID)
	saved, err := s.Load(ctx, head.data.DiagnosticRequestID)
	if err != nil {
		return DiagnosticRecord{}, DiagnosticIntent{}, diagnosticRunError(ctx, "verification_failed", err)
	}
	r := saved.Record()
	want := DiagnosticIntent{c.environment, c.config.InstallationID, head.data.DiagnosticRequestID, release, c.catalog, c.config.PagerDutyRegion, r.Snapshot().Intent.SecretID, true}
	digest, _ := diagnosticIdentity(want)
	if !r.Matches(want) || digest != head.data.DiagnosticIntent {
		return DiagnosticRecord{}, DiagnosticIntent{}, &SetupError{Code: "setup_stale"}
	}
	return r, want, nil
}

func (c setupControls) operatorRecord(ctx context.Context, objects OperatorChallengeObjects, head setupCurrent) (OperatorChallenge, OperatorIntent, error) {
	want, err := NewOperatorIntent(c.environment, c.config.InstallationID, head.data.OperatorRequestID, c.desired)
	if err != nil || operatorIntentDigest(want) != head.data.OperatorIntent {
		return OperatorChallenge{}, OperatorIntent{}, &SetupError{Code: "setup_stale"}
	}
	s, _ := NewOperatorChallengeStore(objects, c.environment, c.config.InstallationID)
	saved, err := s.Load(ctx, head.data.OperatorRequestID)
	if err != nil {
		return OperatorChallenge{}, OperatorIntent{}, operatorRunError(ctx, "verification_failed", err)
	}
	if !saved.Record().Matches(want) {
		return OperatorChallenge{}, OperatorIntent{}, &SetupError{Code: "setup_stale"}
	}
	return saved.Record(), want, nil
}
