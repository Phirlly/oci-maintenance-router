package reconciliation

import (
	"context"
	"oci-maintenance-router/internal/ocistorage"
	"strings"
)

// Load never initializes or interprets a missing current object as a fresh request.
func (s *OperatorChallengeStore) Load(ctx context.Context, request string) (StoredOperatorChallenge, error) {
	if !s.usable() || !operatorLocalID.MatchString(request) {
		return StoredOperatorChallenge{}, &OperatorChallengeStorageError{Code: "invalid_request"}
	}
	ctx, cancel, err := operatorStoreContext(ctx)
	if err != nil {
		return StoredOperatorChallenge{}, err
	}
	defer cancel()
	got, _, failure := s.load(ctx, request)
	if failure != nil {
		return StoredOperatorChallenge{}, failure
	}
	return got, nil
}
func (s *OperatorChallengeStore) load(ctx context.Context, request string) (StoredOperatorChallenge, bool, *OperatorChallengeStorageError) {
	if ctx.Err() != nil {
		return StoredOperatorChallenge{}, false, operatorStorageFailure(ctx, "read_failed", 0, false)
	}
	obj, err := s.objects.Get(ctx, s.name(request))
	if err != nil {
		service, ok := err.(*ocistorage.Error)
		missing := ctx.Err() == nil && ok && service != nil && service.Code == ocistorage.Service && service.Status == 404
		return StoredOperatorChallenge{}, missing, operatorStorageFailure(ctx, "read_failed", baselineStorageStatus(err), false)
	}
	if ctx.Err() != nil {
		return StoredOperatorChallenge{}, false, operatorStorageFailure(ctx, "read_failed", 200, false)
	}
	if !baselineETag(obj.ETag) {
		return StoredOperatorChallenge{}, false, &OperatorChallengeStorageError{Code: "invalid_response", HTTPStatus: 200}
	}
	r, err := DecodeOperatorChallenge(obj.Body, s.environment, s.installation, request)
	if err != nil || ctx.Err() != nil {
		return StoredOperatorChallenge{}, false, operatorStorageFailure(ctx, "invalid_record", 200, false)
	}
	return StoredOperatorChallenge{s, r, strings.Clone(obj.ETag)}, false, nil
}
