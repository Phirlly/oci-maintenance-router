package deployment

import (
	"errors"

	"oci-maintenance-router/internal/ocistorage"
)

var errPinnedContentMismatch = errors.New("pinned content mismatch")

// Only controlled startup evidence reaches logs; provider messages and object
// identities never do. The stage remains the existing top-level result code.
type pinnedReadDiagnostic struct {
	Category   string `json:"category"`
	HTTPStatus int    `json:"httpStatus,omitempty"`
}

func failedPinnedRead(code string, err error) Result {
	result := failed(code)
	result.pinnedRead = describePinnedRead(err)
	return result
}

func describePinnedRead(err error) *pinnedReadDiagnostic {
	diagnostic := &pinnedReadDiagnostic{Category: "unknown"}
	var detail *ocistorage.Error
	if errors.As(err, &detail) {
		// A typed-nil error must not reach Error.Unwrap through errors.Is.
		if detail == nil {
			return diagnostic
		}
		switch detail.Code {
		case ocistorage.InvalidConfiguration, ocistorage.InvalidRequest,
			ocistorage.DeadlineRequired, ocistorage.Canceled, ocistorage.DeadlineExceeded,
			ocistorage.Authentication, ocistorage.Transport, ocistorage.InvalidResponse,
			ocistorage.TooLarge, ocistorage.Conflict, ocistorage.Service:
			diagnostic.Category = string(detail.Code)
			if detail.Status >= 100 && detail.Status <= 599 {
				diagnostic.HTTPStatus = detail.Status
			}
		}
	} else if errors.Is(err, errPinnedContentMismatch) {
		diagnostic.Category = "content_mismatch"
	}
	return diagnostic
}
