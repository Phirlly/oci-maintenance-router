package infra_test

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
)

func closureIntent() map[string]any {
	return map[string]any{"kind": "CLOSE_REVISION", "noticeId": "ocid1.announcement.oc1..example",
		"reviewFingerprint": strings.Repeat("a", 64), "routingGeneration": strings.Repeat("b", 64), "targetId": strings.Repeat("c", 64), "version": 1}
}

func closureCode(v map[string]any) string {
	b, _ := json.Marshal(v)
	return base64.StdEncoding.EncodeToString(b)
}

func closureSettings() map[string]any {
	s := settingsFixture()
	s["mode"] = "PAUSED"
	s["closure_review_code"] = closureCode(closureIntent())
	s["closure_request_number"] = 7
	s["closure_confirm"] = true
	return s
}

func TestClosureRendererConsumesExactExplicitIntent(t *testing.T) {
	values, message, err := render(t, closureSettings())
	if err != nil {
		t.Fatalf("render: %s (%v)", message, err)
	}
	c := renderedConfig(t, values)
	if c.Recovery == nil || c.Recovery.Kind != "CLOSE_REVISION" || c.Recovery.NoticeID != closureIntent()["noticeId"] || c.Recovery.ReviewFingerprint != strings.Repeat("a", 64) || !strings.HasPrefix(c.Recovery.RequestID, "c1-") {
		t.Fatalf("explicit closure intent not rendered: %+v", c.Recovery)
	}
}
