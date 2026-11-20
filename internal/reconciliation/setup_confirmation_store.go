package reconciliation

import (
	"context"
	"oci-maintenance-router/internal/ocistorage"
)

type SetupConfirmationObjects interface {
	Get(context.Context, string) (ocistorage.Object, error)
	Create(context.Context, string, []byte) (string, error)
	HistoryIsEmpty(context.Context, string) (bool, error)
}

type SetupConfirmationStore struct {
	objects                   SetupConfirmationObjects
	environment, installation string
}

func NewSetupConfirmationStore(objects SetupConfirmationObjects, environment, installation string) (*SetupConfirmationStore, error) {
	s := &SetupConfirmationStore{objects, environment, installation}
	if !s.usable() {
		return nil, &SetupConfirmationError{Code: "invalid_request"}
	}
	return s, nil
}

func (s *SetupConfirmationStore) usable() bool {
	return s != nil && baselineObjectsUsable(s.objects, s.environment) && diagnosticID.MatchString(s.installation)
}
func (s *SetupConfirmationStore) name(id string) string {
	return "control/setup-confirmations/" + s.installation + "/" + id + ".json"
}

func (s *SetupConfirmationStore) Create(ctx context.Context, proposal SetupConfirmation) (SetupConfirmation, error) {
	if !s.usable() || proposal.data == nil || proposal.notBefore.IsZero() || proposal.data.EnvironmentID != s.environment || proposal.data.InstallationID != s.installation {
		return SetupConfirmation{}, &SetupConfirmationError{Code: "invalid_request"}
	}
	ctx, cancel, err := baselineStoreContext(ctx)
	if err != nil {
		return SetupConfirmation{}, err
	}
	defer cancel()
	got, missing, err := s.load(ctx, proposal.ID())
	if err == nil {
		return setupConfirmationExisting(ctx, got, proposal, false)
	}
	if !missing {
		return SetupConfirmation{}, err
	}
	empty, err := s.objects.HistoryIsEmpty(ctx, s.name(proposal.ID()))
	if err != nil || ctx.Err() != nil {
		return SetupConfirmation{}, setupConfirmationFailure(ctx, "history_read", baselineStorageStatus(err), false)
	}
	if !empty {
		return SetupConfirmation{}, &SetupConfirmationError{Code: "history_retained"}
	}
	return s.write(ctx, proposal)
}

func (s *SetupConfirmationStore) load(ctx context.Context, id string) (SetupConfirmation, bool, error) {
	if ctx.Err() != nil {
		return SetupConfirmation{}, false, setupConfirmationFailure(ctx, "read_failed", 0, false)
	}
	obj, err := s.objects.Get(ctx, s.name(id))
	if err != nil {
		e, ok := err.(*ocistorage.Error)
		missing := ctx.Err() == nil && ok && e != nil && e.Code == ocistorage.Service && e.Status == 404
		return SetupConfirmation{}, missing, setupConfirmationFailure(ctx, "read_failed", baselineStorageStatus(err), false)
	}
	if ctx.Err() != nil {
		return SetupConfirmation{}, false, setupConfirmationFailure(ctx, "read_failed", 200, false)
	}
	if !baselineETag(obj.ETag) {
		return SetupConfirmation{}, false, &SetupConfirmationError{Code: "invalid_response", HTTPStatus: 200}
	}
	r, err := DecodeSetupConfirmation(obj.Body, s.environment, s.installation, id)
	if err != nil {
		return SetupConfirmation{}, false, &SetupConfirmationError{Code: "invalid_record", HTTPStatus: 200}
	}
	return r, false, nil
}

func setupConfirmationExisting(ctx context.Context, got, proposal SetupConfirmation, uncertain bool) (SetupConfirmation, error) {
	if ctx.Err() != nil {
		return SetupConfirmation{}, setupConfirmationFailure(ctx, "read_failed", 200, uncertain)
	}
	if got.ID() != proposal.ID() || got.confirmedAt().Before(proposal.notBefore) || got.confirmedAt().After(proposal.confirmedAt()) {
		return SetupConfirmation{}, &SetupConfirmationError{Code: "identity_conflict", HTTPStatus: 200, WriteUncertain: uncertain}
	}
	return got, nil
}
