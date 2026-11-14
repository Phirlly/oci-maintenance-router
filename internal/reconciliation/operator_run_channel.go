package reconciliation

import (
	"context"
)

func (r *operatorRun) channel() (operatorObservation, error) {
	if err := r.reserve(); err != nil {
		return operatorObservation{}, operatorRunError(r.ctx, "channel_read", err)
	}
	ctx, cancel := context.WithTimeout(r.ctx, discoveryReserve)
	got, err := r.owner.read(ctx)
	late := ctx.Err()
	cancel()
	if err != nil || late != nil {
		return operatorObservation{}, operatorChannelReadError(r.ctx, err)
	}
	if got.channel != r.owner.desired || got.publish == nil {
		return operatorObservation{}, &OperatorRunError{Code: "channel_changed"}
	}
	return got, nil
}
