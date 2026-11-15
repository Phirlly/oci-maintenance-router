package deployment

import (
	"context"
	"testing"

	"oci-maintenance-router/internal/reconciliation"
)

func TestRecoveryReviewReturnsOnlyControlledFailureCodes(t *testing.T) {
	for _, kind := range []string{"source_changed", "private provider response /resource", "nil"} {
		t.Run(kind, func(t *testing.T) {
			f := reviewLiveFixture(t)
			f.h.compose = func(Settings, Store, pinnedInputs, string) (operations, error) {
				return operations{recoveryReview: func(context.Context, command) (*RecoveryReviewPage, error) {
					var err *reconciliation.RecoveryReviewError
					if kind != "nil" {
						err = &reconciliation.RecoveryReviewError{Code: kind}
					}
					return nil, err
				}}, nil
			}
			got := invokeOperational(t, f.h, recoveryReviewRequest(f.notice.Key().NoticeID, "APPROVE_REVISION", f.notice.Key().Digest, ""))
			want := "operation_failed"
			if kind == "source_changed" {
				want = kind
			}
			if got.Status != "not-ready" || got.Code != want || got.RecoveryReview != nil {
				t.Fatal("review error leaked private data or claimed success", got)
			}
		})
	}
}
