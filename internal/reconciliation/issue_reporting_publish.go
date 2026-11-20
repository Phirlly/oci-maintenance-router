package reconciliation

import "context"

func (r *reportingRun) publish() error {
	state := cloneReport(r.saved.state)
	claimed, err := claimReport(&state, r.token, r.lease.manager.now(), r.deadline)
	if err != nil || !claimed {
		return err
	}
	if err = r.owner.verifyReceipt(r.ctx, r.lease); err != nil {
		return &ReportingError{"receipt_unverified"}
	}
	if err = r.guard(); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(r.ctx, discoveryReserve)
	channel, err := r.owner.read(ctx)
	late := ctx.Err()
	cancel()
	if err != nil || late != nil || channel.channel != r.owner.desired || channel.publish == nil {
		return &ReportingError{"channel_unavailable"}
	}
	if err = r.save(state, false); err != nil {
		return err
	}
	if err = r.guard(); err != nil {
		return err
	}
	r.result.Dispatched = true
	publication, failure := channel.publish(r.ctx, r.saved.state.Pending.Payload)
	status, messageID := 0, ""
	if publication.Status >= 100 && publication.Status <= 599 {
		status = publication.Status
	}
	if failure == nil && (status == 200 || status == 202) && operatorOpaqueText(publication.MessageID, 256) {
		messageID = publication.MessageID
	}
	state = cloneReport(r.saved.state)
	r.observed = r.lease.manager.now()
	if err = finishReport(&state, r.token, status, messageID, r.observed); err != nil {
		return err
	}
	if err = r.save(state, true); err != nil {
		return err
	}
	r.result.Accepted = messageID != ""
	if !r.result.Accepted {
		return &ReportingError{"publication_unconfirmed"}
	}
	return nil
}
