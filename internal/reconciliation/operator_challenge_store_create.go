package reconciliation

import "context"

// Create preserves the first same-intent record, even when another operation
// claimed it or it has progressed. The caller must not infer send ownership from
// success. Ordinary deletion cannot reset a request while its history remains.
func (s *OperatorChallengeStore) Create(ctx context.Context, initial OperatorChallenge) (StoredOperatorChallenge, error) {
	if !s.usable() || initial.data == nil || initial.parent != nil || initial.data.State != OperatorPublishing || initial.data.Intent.EnvironmentID != s.environment || initial.data.Intent.InstallationID != s.installation {
		return StoredOperatorChallenge{}, &OperatorChallengeStorageError{Code: "invalid_request"}
	}
	ctx, cancel, err := operatorStoreContext(ctx)
	if err != nil {
		return StoredOperatorChallenge{}, err
	}
	defer cancel()
	request := initial.data.Intent.RequestID
	got, missing, failure := s.load(ctx, request)
	if failure == nil {
		return operatorExisting(ctx, got, initial)
	}
	if !missing {
		return StoredOperatorChallenge{}, failure
	}
	if ctx.Err() != nil {
		return StoredOperatorChallenge{}, operatorStorageFailure(ctx, "history_read", 0, false)
	}
	empty, err := s.objects.HistoryIsEmpty(ctx, s.name(request))
	if err != nil || ctx.Err() != nil {
		return StoredOperatorChallenge{}, operatorStorageFailure(ctx, "history_read", baselineStorageStatus(err), false)
	}
	if !empty {
		return StoredOperatorChallenge{}, &OperatorChallengeStorageError{Code: "history_retained"}
	}
	return s.write(ctx, "", initial)
}
func operatorExisting(ctx context.Context, got StoredOperatorChallenge, initial OperatorChallenge) (StoredOperatorChallenge, error) {
	if ctx.Err() != nil {
		return StoredOperatorChallenge{}, operatorStorageFailure(ctx, "read_failed", 200, false)
	}
	if !got.record.Matches(initial.data.Intent) {
		return StoredOperatorChallenge{}, &OperatorChallengeStorageError{Code: "identity_conflict", HTTPStatus: 200}
	}
	return got, nil
}
