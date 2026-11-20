package reconciliation

import "context"

// One conditional create, followed only by an exact readback when permitted by
// the existing adapter contract. No write retry, replacement or history removal.
func (s *SetupConfirmationStore) write(ctx context.Context, proposal SetupConfirmation) (SetupConfirmation, error) {
	if ctx.Err() != nil {
		return SetupConfirmation{}, setupConfirmationFailure(ctx, "write_failed", 0, false)
	}
	body, _ := proposal.Encode()
	tag, err := s.objects.Create(ctx, s.name(proposal.ID()), body)
	if blocked, ok := err.(*setupConfirmationBlocked); ok && blocked != nil {
		return SetupConfirmation{}, blocked.failure
	}
	confirm, uncertain, status := false, true, 200
	if err != nil {
		confirm, uncertain = baselineWriteDisposition(err)
		status = baselineStorageStatus(err)
	} else {
		confirm = !baselineETag(tag)
	}
	if ctx.Err() != nil || err != nil && !confirm {
		return SetupConfirmation{}, setupConfirmationFailure(ctx, "write_failed", status, uncertain)
	}
	if !confirm {
		return proposal, nil
	}
	got, _, failure := s.load(ctx, proposal.ID())
	if failure != nil {
		// load returns only controlled owned errors.
		e := failure.(*SetupConfirmationError)
		e.WriteUncertain = uncertain
		return SetupConfirmation{}, e
	}
	return setupConfirmationExisting(ctx, got, proposal, uncertain)
}
