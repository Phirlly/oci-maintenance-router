package reconciliation

import (
	"context"
	"strings"
)

func (s *setupCurrentStore) create(ctx context.Context, proposal setupCurrent) (storedSetupCurrent, error) {
	if !s.validProposal(proposal) || proposal.parent != nil || proposal.data.Confirmation != nil {
		return storedSetupCurrent{}, &SetupError{Code: "invalid_request"}
	}
	ctx, cancel, err := baselineStoreContext(ctx)
	if err != nil {
		return storedSetupCurrent{}, err
	}
	defer cancel()
	got, missing, err := s.read(ctx)
	if err == nil {
		if !sameSetupCurrent(got.record, proposal) {
			return storedSetupCurrent{}, &SetupError{Code: "identity_conflict"}
		}
		return got, nil
	}
	if !missing {
		return storedSetupCurrent{}, err
	}
	empty, err := s.objects.HistoryIsEmpty(ctx, s.name())
	if err != nil || ctx.Err() != nil {
		return storedSetupCurrent{}, setupFailure(ctx, "history_read", baselineStorageStatus(err), false)
	}
	if !empty {
		return storedSetupCurrent{}, &SetupError{Code: "history_retained"}
	}
	return s.write(ctx, "", proposal)
}

func (s *setupCurrentStore) save(ctx context.Context, previous storedSetupCurrent, proposal setupCurrent) (storedSetupCurrent, error) {
	if !s.validProposal(proposal) || previous.store != s || proposal.parent == nil || proposal.parent != previous.record.data || !baselineETag(previous.etag) {
		return storedSetupCurrent{}, &SetupError{Code: "invalid_request"}
	}
	ctx, cancel, err := baselineStoreContext(ctx)
	if err != nil {
		return storedSetupCurrent{}, err
	}
	defer cancel()
	return s.write(ctx, previous.etag, proposal)
}

func (s *setupCurrentStore) write(ctx context.Context, tag string, proposal setupCurrent) (storedSetupCurrent, error) {
	if ctx.Err() != nil {
		return storedSetupCurrent{}, setupFailure(ctx, "write_failed", 0, false)
	}
	body, _ := proposal.encode()
	var etag string
	var err error
	if tag == "" {
		etag, err = s.objects.Create(ctx, s.name(), body)
	} else {
		etag, err = s.objects.Replace(ctx, s.name(), tag, body)
	}
	if blocked, ok := err.(*setupWriteBlocked); ok && blocked != nil {
		return storedSetupCurrent{}, blocked.failure
	}
	confirm, uncertain, status := !baselineETag(etag), true, 200
	if err != nil {
		confirm, uncertain = baselineWriteDisposition(err)
		status = baselineStorageStatus(err)
	}
	if ctx.Err() != nil || err != nil && !confirm {
		return storedSetupCurrent{}, setupFailure(ctx, "write_failed", status, uncertain)
	}
	if !confirm {
		return storedSetupCurrent{s, proposal, strings.Clone(etag)}, nil
	}
	got, _, err := s.read(ctx)
	if err != nil {
		if e, ok := err.(*SetupError); ok && e != nil {
			e.WriteUncertain = uncertain
			return storedSetupCurrent{}, e
		}
		return storedSetupCurrent{}, setupFailure(ctx, "read_failed", 0, uncertain)
	}
	if !sameSetupCurrent(got.record, proposal) {
		return storedSetupCurrent{}, &SetupError{Code: "identity_conflict", HTTPStatus: 200, WriteUncertain: uncertain}
	}
	return got, nil
}
