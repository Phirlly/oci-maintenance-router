package deployment

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"oci-maintenance-router/internal/reconciliation"
)

func TestRecoveryRejectsWrongAuthorityBeforeOwnership(t *testing.T) {
	for _, kind := range []string{"configuration", "release", "request", "PREVIEW", "PAUSED", "TEST", "absent", "CLOSE_REVISION"} {
		t.Run(kind, func(t *testing.T) {
			h, s, objects, _ := liveOperationFixture(t)
			s = configureOperational(t, h, s, objects, func(v map[string]any) {
				v["recovery"] = sendingRecoveryIntent("RETRY_BATCH")
				switch kind {
				case "PREVIEW", "PAUSED", "TEST":
					v["mode"] = kind
				case "absent":
					delete(v, "recovery")
				case "CLOSE_REVISION":
					q := v["recovery"].(map[string]any)
					q["kind"] = "CLOSE_REVISION"
					delete(q, "componentFingerprint")
				}
			})
			request := s
			id := "r1-reviewed"
			switch kind {
			case "configuration":
				request.ConfigurationSHA256 = strings.Repeat("f", 64)
			case "release":
				request.ReleaseSHA256 = strings.Repeat("f", 64)
			case "request":
				id = "another"
			}
			before := objects.version
			h.compose = func(Settings, Store, pinnedInputs, string) (operations, error) {
				t.Fatal("invalid request composed work")
				return operations{}, nil
			}
			got := closureResponse(t, h, recoverInvocation(request, id))
			want := "recovery_intent_mismatch"
			if kind == "configuration" || kind == "release" {
				want = "request_mismatch"
			}
			if got["status"] != "not-ready" || got["code"] != want || objects.version != before {
				t.Fatal("wrong authority acquired ownership", got)
			}
		})
	}
}

func TestRecoveryOverlapNeverClaimsSuccessOrReleasesAnotherOwner(t *testing.T) {
	h, s, objects, _ := liveOperationFixture(t)
	s = configureOperational(t, h, s, objects, func(v map[string]any) { v["recovery"] = sendingRecoveryIntent("RETRY_BATCH") })
	p, failure := loadPinned(operationalContext(t), s, objects)
	if failure.Status != "" {
		t.Fatal(failure)
	}
	manager, err := reconciliation.NewLeaseManager(objects, p.environment)
	if err != nil {
		t.Fatal(err)
	}
	lease, err := manager.Acquire(operationalContext(t))
	if err != nil || lease == nil {
		t.Fatal(err)
	}
	before := objects.version
	out := httptest.NewRecorder()
	h.Serve(operationalContext(t), strings.NewReader(recoverInvocation(s, "r1-reviewed")), out)
	if out.Code != 400 || !strings.Contains(out.Body.String(), `"status":"coalesced"`) || objects.version != before {
		t.Fatal("overlap verified or changed ownership", out.Body)
	}
	if err = lease.Check(operationalContext(t), time.Second); err != nil {
		t.Fatal("other owner released", err)
	}
}

func TestRecoveryRequiresExactSuccessfulRetainedOutcome(t *testing.T) {
	for _, kind := range []string{"absent", "foreign", "REJECTED", "CONSUMED", "REVIEW_REQUIRED", "reason", "invalidation"} {
		t.Run(kind, func(t *testing.T) {
			h, s, objects, _ := liveOperationFixture(t)
			s = configureOperational(t, h, s, objects, func(v map[string]any) { v["recovery"] = sendingRecoveryIntent("APPROVE_REVISION") })
			h.compose = func(Settings, Store, pinnedInputs, string) (operations, error) {
				ops := successfulReporting()
				ops.process = func(context.Context, *reconciliation.Lease) (reconciliation.ProcessingSnapshot, error) {
					r := &reconciliation.ProcessingRecovery{RequestID: "r1-reviewed", Status: "APPROVED"}
					switch kind {
					case "absent":
						r = nil
					case "foreign":
						r.RequestID = "other"
					case "reason":
						r.Reason = "stale_review"
					case "invalidation":
						r.InvalidationReason = "source_changed"
					default:
						r.Status = kind
					}
					return reconciliation.ProcessingSnapshot{Completed: true, Recovery: r}, nil
				}
				return ops, nil
			}
			got := invokeOperational(t, h, recoverInvocation(s, "r1-reviewed"))
			if got.Status != "not-ready" || got.Code != "recovery_not_completed" || got.ScanCompleted {
				t.Fatal("unconfirmed recovery accepted", kind, got)
			}
		})
	}
}
