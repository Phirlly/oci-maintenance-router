package deployment

import (
	"net/http"
	"strings"
	"testing"
)

func TestNotificationBodyIsOpaqueAndBounded(t *testing.T) {
	for _, optional := range []string{"neither", "topic", "type", "both"} {
		t.Run(optional, func(t *testing.T) {
			headers := functionNotificationHeaders()
			if optional == "topic" || optional == "both" {
				headers.Set("X-OCI-NS-TopicOcid", intakeHeaders().Get("X-OCI-NS-TopicOcid"))
			}
			if optional == "type" || optional == "both" {
				headers.Set("X-OCI-NS-MessageType", "synthetic-type")
			}
			for _, body := range []string{"", "not JSON", `{"operation":"deployment-check"}`, strings.Repeat("x", 65536)} {
				h, s, objects, _ := operationalFixture(t)
				configureOperational(t, h, s, objects, func(v map[string]any) { v["mode"] = "PAUSED" })
				got := automaticResponse(t, h, operationalContext(t), headers, strings.NewReader(body))
				if got.Status != "idle" || got.Operation != "reconcile" || got.CheckAttemptID != "" || objects.version != 0 {
					t.Fatalf("opaque notification interpreted or rejected: %+v", got)
				}
			}
		})
	}
}

func TestNotificationMetadataCannotFallBackToCommands(t *testing.T) {
	for _, kind := range []string{"missing-id", "duplicate-topic", "duplicate-case", "empty", "oversized", "control", "invalid-utf8", "wrong-topic", "only-url"} {
		t.Run(kind, func(t *testing.T) {
			h, s, objects, _ := operationalFixture(t)
			headers := intakeHeaders()
			switch kind {
			case "missing-id":
				headers.Del("X-OCI-NS-MessageId")
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
				headers.Del("X-OCI-NS-MessageType")
			case "only-url":
				headers = http.Header{"X-OCI-NS-SigningCertURL": []string{"https://example.invalid/private"}}
			}
			got := automaticResponse(t, h, operationalContext(t), headers, strings.NewReader(invocation(s)))
			if got.Status != "not-ready" || objects.version != 0 || got.BaselinePrepared || got.CheckAttemptID != "" {
				t.Fatalf("metadata fell back to privileged command: %+v", got)
			}
			if kind == "wrong-topic" && got.Code != "intake_mismatch" {
				t.Fatalf("supplied topic was not checked against configuration: %+v", got)
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
