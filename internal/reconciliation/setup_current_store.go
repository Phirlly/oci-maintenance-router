package reconciliation

import (
	"bytes"
	"context"
	"strings"

	"oci-maintenance-router/internal/ocistorage"
)

type setupCurrentObjects interface {
	PreparedHistoryObjects
	HistoryIsEmpty(context.Context, string) (bool, error)
}

type setupCurrentStore struct {
	objects                   setupCurrentObjects
	environment, installation string
}

type storedSetupCurrent struct {
	store  *setupCurrentStore
	record setupCurrent
	etag   string
}

func newSetupCurrentStore(objects setupCurrentObjects, environment, installation string) (*setupCurrentStore, error) {
	s := &setupCurrentStore{objects, environment, installation}
	if !s.usable() {
		return nil, &SetupError{Code: "invalid_request"}
	}
	return s, nil
}

func (s *setupCurrentStore) usable() bool {
	return s != nil && baselineObjectsUsable(s.objects, s.environment) && diagnosticID.MatchString(s.installation)
}

func (s *setupCurrentStore) name() string { return "control/setup-current/" + s.installation + ".json" }

func (s *setupCurrentStore) load(ctx context.Context) (storedSetupCurrent, bool, error) {
	if !s.usable() {
		return storedSetupCurrent{}, false, &SetupError{Code: "invalid_request"}
	}
	ctx, cancel, err := baselineStoreContext(ctx)
	if err != nil {
		return storedSetupCurrent{}, false, err
	}
	defer cancel()
	return s.read(ctx)
}

func (s *setupCurrentStore) read(ctx context.Context) (storedSetupCurrent, bool, error) {
	if ctx.Err() != nil {
		return storedSetupCurrent{}, false, setupFailure(ctx, "read_failed", 0, false)
	}
	obj, err := s.objects.Get(ctx, s.name())
	if err != nil {
		e, ok := err.(*ocistorage.Error)
		missing := ctx.Err() == nil && ok && e != nil && e.Code == ocistorage.Service && e.Status == 404
		return storedSetupCurrent{}, missing, setupFailure(ctx, "read_failed", baselineStorageStatus(err), false)
	}
	if ctx.Err() != nil {
		return storedSetupCurrent{}, false, setupFailure(ctx, "read_failed", 200, false)
	}
	if !baselineETag(obj.ETag) {
		return storedSetupCurrent{}, false, &SetupError{Code: "invalid_response", HTTPStatus: 200}
	}
	r, err := decodeSetupCurrent(obj.Body, s.environment, s.installation)
	if err != nil || ctx.Err() != nil {
		return storedSetupCurrent{}, false, setupFailure(ctx, "invalid_record", 200, false)
	}
	return storedSetupCurrent{s, r, strings.Clone(obj.ETag)}, false, nil
}

func (s *setupCurrentStore) validProposal(proposal setupCurrent) bool {
	if !s.usable() || proposal.data == nil {
		return false
	}
	body, err := proposal.encode()
	if err != nil {
		return false
	}
	_, err = decodeSetupCurrent(body, s.environment, s.installation)
	return err == nil
}

func sameSetupCurrent(a, b setupCurrent) bool {
	ab, ae := a.encode()
	bb, be := b.encode()
	return ae == nil && be == nil && bytes.Equal(ab, bb)
}
