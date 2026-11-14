package reconciliation

import (
	"context"
	"oci-maintenance-router/internal/ocinotifications"
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
		failure := operatorRunError(r.ctx, "channel_read", nil)
		if e, ok := err.(*ocinotifications.Error); ok && e != nil && e.Status >= 100 && e.Status <= 599 {
			failure.HTTPStatus = e.Status
		}
		return operatorObservation{}, failure
	}
	if got.channel != r.owner.desired || got.publish == nil {
		return operatorObservation{}, &OperatorRunError{Code: "channel_changed"}
	}
	return got, nil
}
