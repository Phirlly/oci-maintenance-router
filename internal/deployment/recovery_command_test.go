package deployment

import (
	"encoding/json"
	"strings"
	"testing"
)

func recoveryReviewRequest(id, kind, target, cursor string) string {
	v := map[string]string{"operation": "review-recovery", "noticeId": id}
	if kind != "" {
		v["kind"], v["targetId"] = kind, target
	}
	if cursor != "" {
		v["cursor"] = cursor
	}
	body, _ := json.Marshal(v)
	return string(body)
}

func recoverInvocation(s Settings, id string) string {
	body, _ := json.Marshal(map[string]string{"operation": "recover-work", "checkAttemptId": testAttemptID, "configurationSha256": s.ConfigurationSHA256, "releaseSha256": s.ReleaseSHA256, "recoveryRequestId": id})
	return string(body)
}

func TestSendingRecoveryCommandsKeepStrictBoundaries(t *testing.T) {
	prefix := "ocid1.announcement.oc1.."
	id := prefix + strings.Repeat("a", 4096-len(prefix))
	for _, kind := range []string{"", "RETRY_BATCH", "APPROVE_REVISION"} {
		if _, err := parseCommand([]byte(recoveryReviewRequest(id, kind, strings.Repeat("a", 64), ""))); err != nil {
			t.Fatal("valid review", kind, err)
		}
	}
	for _, body := range []string{
		`{"operation":"review-recovery","noticeId":"ocid1.announcement.oc1..sample","kind":"RETRY_BATCH"}`,
		recoveryReviewRequest(id, "CLOSE_BATCH", strings.Repeat("a", 64), ""),
		recoveryReviewRequest(id, "RETRY_BATCH", "wrong", ""),
		recoveryReviewRequest(id, "RETRY_BATCH", strings.Repeat("a", 64), "bad"),
	} {
		if _, err := parseCommand([]byte(body)); err == nil {
			t.Fatal("invalid review accepted", body)
		}
	}
	s, _ := fixture(t)
	if _, err := parseCommand([]byte(recoverInvocation(s, "r1-example"))); err != nil {
		t.Fatal("valid execution", err)
	}
}
