package deployment

import (
	"encoding/json"
	"strings"
	"testing"
)

func closureReviewRequest(id, kind, target, cursor string) string {
	v := map[string]string{"operation": "review-closure", "noticeId": id}
	if kind != "" {
		v["kind"], v["targetId"] = kind, target
	}
	if cursor != "" {
		v["cursor"] = cursor
	}
	b, _ := json.Marshal(v)
	return string(b)
}

func TestClosureCommandsSelectOnlyBoundedExplicitWork(t *testing.T) {
	for _, input := range []string{
		closureReviewRequest("ocid1.announcement.oc1..example", "", "", ""),
		closureReviewRequest("ocid1.announcement.oc1..example", "CLOSE_REVISION", strings.Repeat("a", 64), ""),
		`{"operation":"close-work","configurationSha256":"` + strings.Repeat("a", 64) + `","releaseSha256":"` + strings.Repeat("b", 64) + `","checkAttemptId":"12345678-1234-1234-1234-123456789abc","recoveryRequestId":"c1-example"}`,
	} {
		if _, err := parseCommand([]byte(input)); err != nil {
			t.Fatalf("explicit closure request refused: %s", input)
		}
	}
}

func TestClosureReviewRejectsIncompleteOrForeignAuthority(t *testing.T) {
	for _, extra := range []string{
		`,"kind":"CLOSE_BATCH"`, `,"targetId":"` + strings.Repeat("a", 64) + `"`,
		`,"kind":"RETRY_BATCH","targetId":"` + strings.Repeat("a", 64) + `"`,
		`,"kind":"CLOSE_BATCH","targetId":"bad"`, `,"recoveryRequestId":"request"`,
		`,"noticeId":"ocid1.announcement.oc1..duplicate"`, `,"kind":null`,
	} {
		body := `{"operation":"review-closure","noticeId":"ocid1.announcement.oc1..example"` + extra + `}`
		if _, err := parseCommand([]byte(body)); err == nil {
			t.Fatalf("accepted invalid closure review: %s", body)
		}
	}
}
