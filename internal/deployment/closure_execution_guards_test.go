package deployment

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"oci-maintenance-router/internal/noticeledger"
)

func TestClosureRefusesUnpinnedForeignOrSendingIntent(t *testing.T) {
	for _, kind := range []string{"configuration", "release", "request", "PREVIEW", "TEST", "LIVE", "absent", "APPROVE_REVISION"} {
		t.Run(kind, func(t *testing.T) {
			h, s, objects, env, _ := retainedTestFixture(t, false)
			s = configureOperational(t, h, s, objects, func(v map[string]any) { v["mode"] = "PAUSED" })
			s, _, _ = configureRecovery(t, h, s, objects, env, noticeledger.CloseRevision)
			switch kind {
			case "PREVIEW", "TEST":
				s = configureOperational(t, h, s, objects, func(v map[string]any) { v["mode"] = kind })
			case "LIVE":
				s = configureOperational(t, h, s, objects, func(v map[string]any) {
					v["mode"] = "LIVE"
					v["diagnostic"].(map[string]any)["receiptCode"] = "OMR-" + strings.Repeat("a", 32)
					v["operatorTest"] = map[string]any{"requestId": "operator-1", "subscriptionId": "ocid1.onssubscription.oc1.iad.operator", "endpoint": "operator@example.invalid", "receiptCode": "OMR-OP-" + strings.Repeat("b", 32)}
				})
			case "absent":
				s = configureOperational(t, h, s, objects, func(v map[string]any) { delete(v, "recovery") })
			case "APPROVE_REVISION":
				s = configureOperational(t, h, s, objects, func(v map[string]any) {
					q := v["recovery"].(map[string]any)
					q["kind"] = "APPROVE_REVISION"
					q["componentFingerprint"] = strings.Repeat("a", 64)
				})
			}
			request := s
			id := "recovery-1"
			if kind == "configuration" {
				request.ConfigurationSHA256 = strings.Repeat("f", 64)
			}
			if kind == "release" {
				request.ReleaseSHA256 = strings.Repeat("f", 64)
			}
			if kind == "request" {
				id = "another"
			}
			before := businessObjects(objects)
			got := closureResponse(t, h, closeInvocation(request, id))
			want := "closure_intent_mismatch"
			if kind == "configuration" || kind == "release" {
				want = "request_mismatch"
			}
			if got["status"] != "not-ready" || got["code"] != want || !reflect.DeepEqual(before, businessObjects(objects)) {
				t.Fatal("invalid authority altered saved work", got)
			}
		})
	}
}

func TestClosureCannotReportSuccessAfterFinalGuardOrCleanupFailure(t *testing.T) {
	for _, kind := range []string{"configuration", "cleanup"} {
		t.Run(kind, func(t *testing.T) {
			h, s, objects, env, _ := retainedTestFixture(t, false)
			s = configureOperational(t, h, s, objects, func(v map[string]any) { v["mode"] = "PAUSED" })
			s, _, _ = configureRecovery(t, h, s, objects, env, noticeledger.CloseRevision)
			written := false
			objects.before = func(_ context.Context, method, name string) error {
				if method == "REPLACE" && strings.HasPrefix(name, "notices/") {
					written = true
				}
				if written && kind == "configuration" {
					objects.pinned.config = append(objects.pinned.config, ' ')
				}
				if written && kind == "cleanup" && method == "REPLACE" && name == "control/lease.json" {
					return errors.New("private")
				}
				return nil
			}
			got := invokeOperational(t, h, closeInvocation(s, "recovery-1"))
			if !written || got.Status != "not-ready" {
				t.Fatal("failed closure reported successful", got)
			}
		})
	}
}
