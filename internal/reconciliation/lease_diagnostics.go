package reconciliation

import "oci-maintenance-router/internal/ocistorage"

// LeaseWriteDiagnostic preserves only the initiating uncertain write's safe
// category/status when confirmation fails. It never describes a confirmed write.
type LeaseWriteDiagnostic struct {
	Category   string
	HTTPStatus int
}

func leaseStorageCategory(code ocistorage.Code) string {
	switch code {
	case ocistorage.InvalidConfiguration, ocistorage.InvalidRequest,
		ocistorage.DeadlineRequired, ocistorage.Canceled, ocistorage.DeadlineExceeded,
		ocistorage.Authentication, ocistorage.Transport, ocistorage.InvalidResponse,
		ocistorage.TooLarge, ocistorage.Conflict, ocistorage.Service:
		return string(code)
	}
	return "unknown"
}

func leaseRequestEvidence(err error, request string, prior *LeaseWriteDiagnostic) error {
	detail, ok := err.(*LeaseError)
	if !ok || detail == nil {
		return err
	}
	copy := *detail
	copy.Request = request
	if prior != nil {
		copy.WriteUncertain = true
		copy.PriorWrite = prior
	}
	return &copy
}

func uncertainLeaseWrite(detail *ocistorage.Error) *LeaseWriteDiagnostic {
	result := &LeaseWriteDiagnostic{Category: leaseStorageCategory(detail.Code)}
	if result.Category != "unknown" && detail.Status >= 100 && detail.Status <= 599 {
		result.HTTPStatus = detail.Status
	}
	return result
}
