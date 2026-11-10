package reconciliation

import (
	"context"
	"errors"
	"strings"

	"oci-maintenance-router/internal/ocistorage"
)

func (m *LeaseManager) load(ctx context.Context) (leaseRecord, string, error) {
	object, err := m.objects.Get(ctx, workerLeaseName)
	if err != nil || ctx.Err() != nil {
		return leaseRecord{}, "", leaseError(ctx, "read_failed", err)
	}
	if object.ETag == "" || len(object.ETag) > 1024 {
		return leaseRecord{}, "", &LeaseError{Code: "invalid_response"}
	}
	record, err := decodeLease(object.Body, m.environmentID)
	if err != nil {
		return leaseRecord{}, "", err
	}
	return record, strings.Clone(object.ETag), nil
}
func (m *LeaseManager) confirm(ctx context.Context, want leaseRecord) (string, error) {
	got, tag, err := m.load(ctx)
	if err != nil {
		return "", err
	}
	if got != want {
		return "", &LeaseError{Code: "ownership_lost"}
	}
	return tag, nil
}
func (m *LeaseManager) write(ctx context.Context, record leaseRecord, previous string) (string, error) {
	var tag string
	var err error
	if previous == "" {
		tag, err = m.objects.Create(ctx, workerLeaseName, record.bytes())
	} else {
		tag, err = m.objects.Replace(ctx, workerLeaseName, previous, record.bytes())
	}
	if ctx.Err() != nil {
		return "", leaseError(ctx, "write_failed", err)
	}
	if err == nil {
		if tag == "" || len(tag) > 1024 {
			return "", &LeaseError{Code: "invalid_response"}
		}
		return strings.Clone(tag), nil
	}
	var detail *ocistorage.Error
	if errors.As(err, &detail) && detail != nil {
		if detail.Code == ocistorage.Conflict && detail.Status == 412 && !detail.WriteUncertain {
			return "", leaseError(ctx, "conflict", err)
		}
		rejected := detail.Code == ocistorage.Service && detail.Status >= 400 && detail.Status < 500
		if detail.WriteUncertain && !rejected {
			return m.confirm(ctx, record)
		}
	}
	return "", leaseError(ctx, "write_failed", err)
}
