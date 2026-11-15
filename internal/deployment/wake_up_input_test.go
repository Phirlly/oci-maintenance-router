package deployment

import (
	"net/http"
	"strings"
	"testing"
)

func TestNotificationBodyIsOpaqueAndBounded(t *testing.T) {
	for _, body := range []string{"", "not JSON", `{"operation":"deployment-check"}`, strings.Repeat("x", 65536)} {
		h, s, objects, _ := operationalFixture(t)
		configureOperational(t, h, s, objects, func(v map[string]any) { v["mode"] = "PAUSED" })
		got := automaticResponse(t, h, operationalContext(t), intakeHeaders(), strings.NewReader(body))
		if got.Status != "idle" || objects.version != 0 {
			t.Fatalf("opaque notification interpreted or rejected: %+v", got)
		}
	}
}

func TestNotificationMetadataCannotFallBackToCommands(t *testing.T) {
	for _, kind := range []string{"missing-topic", "missing-id", "missing-type", "duplicate-topic", "duplicate-case", "empty", "oversized", "control", "invalid-utf8", "wrong-topic", "only-url"} {
		t.Run(kind, func(t *testing.T) {
			h, s, objects, _ := operationalFixture(t)
			headers := intakeHeaders()
			switch kind {
			case "missing-topic":
				headers.Del("X-OCI-NS-TopicOcid")
			case "missing-id":
				headers.Del("X-OCI-NS-MessageId")
			case "missing-type":
				headers.Del("X-OCI-NS-MessageType")
			case "duplicate-topic":
				headers.Add("X-OCI-NS-TopicOcid", headers.Get("X-OCI-NS-TopicOcid"))
			case "duplicate-case":
				headers["x-oci-ns-messageid"] = []string{"another"}
			case "empty":
				headers.Set("X-OCI-NS-MessageId", "")
			case "oversized":
				headers.Set("X-OCI-NS-MessageType", strings.Repeat("t", 129))
			case "control":
				headers.Set("X-OCI-NS-MessageId", "private\nvalue")
			case "invalid-utf8":
				headers.Set("X-OCI-NS-MessageType", "\xff")
			case "wrong-topic":
				headers.Set("X-OCI-NS-TopicOcid", "ocid1.onstopic.oc1.iad.operator")
			case "only-url":
				headers = http.Header{"X-OCI-NS-SigningCertURL": []string{"https://example.invalid/private"}}
			}
			got := automaticResponse(t, h, operationalContext(t), headers, strings.NewReader(invocation(s)))
			if got.Status != "not-ready" || objects.version != 0 || got.BaselinePrepared || got.CheckAttemptID != "" {
				t.Fatalf("metadata fell back to privileged command: %+v", got)
			}
		})
	}
}

func TestWakeUpCommandRetainsStrictManualSyntax(t *testing.T) {
	for _, body := range []string{`{"operation":"wake-up","mode":"LIVE"}`, `{"operation":"wake-up","operation":"reconcile"}`, `{"operation":"wake-up","noticeId":"private"}`, `{"operation":"wake-up"}{}`, `{"operation":"wake-up"}` + strings.Repeat(" ", 1024)} {
		if _, err := parseCommand([]byte(body)); err == nil {
			t.Fatal("extended automatic command accepted")
		}
	}
}
