package deployment

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestFDKNotificationHeadersSelectWakeUpInsteadOfBodyCommand(t *testing.T) {
	h, s, objects, _ := operationalFixture(t)
	s = configureOperational(t, h, s, objects, func(v map[string]any) { v["mode"] = "PAUSED" })
	var logs bytes.Buffer
	h.logger = log.New(&logs, "", 0)
	client := protocolClient(t, h)
	for _, kind := range []string{"ordinary", "http-trigger", "function-notification", "http-function-notification", "duplicate", "wrong-topic"} {
		t.Run(kind, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodPost, "http://socket/call", strings.NewReader(invocation(s)))
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Fn-Deadline", time.Now().Add(time.Minute).UTC().Format(time.RFC3339Nano))
			prefix := ""
			if kind == "http-trigger" || kind == "http-function-notification" {
				prefix = "Fn-Http-H-"
				req.Header.Set("Fn-Intent", "httprequest")
			}
			headers := intakeHeaders()
			if kind == "function-notification" || kind == "http-function-notification" {
				headers = functionNotificationHeaders()
			}
			for key, values := range headers {
				req.Header[prefix+key] = values
			}
			if kind == "duplicate" {
				req.Header.Add("X-OCI-NS-TopicOcid", "ocid1.onstopic.oc1.iad.intake")
			}
			if kind == "wrong-topic" {
				req.Header.Set("X-OCI-NS-TopicOcid", "ocid1.onstopic.oc1.iad.operator")
			}
			resp, err := client.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			body, err := io.ReadAll(io.LimitReader(resp.Body, 4097))
			closeErr := resp.Body.Close()
			var got OperationalResult
			if err != nil || closeErr != nil || json.Unmarshal(body, &got) != nil || len(body) > 4096 || resp.StatusCode != http.StatusOK || resp.Header.Get("Fn-Fdk-Version") == "" {
				t.Fatal("invalid actual FDK response", string(body), err, closeErr)
			}
			want := "idle"
			if kind == "duplicate" || kind == "wrong-topic" {
				want = "not-ready"
			}
			if got.Status != want || got.ScanCompleted || got.CheckAttemptID != "" || objects.version != 0 {
				t.Fatalf("FDK input bypassed wake-up boundary: %+v", got)
			}
		})
	}
	for _, private := range []string{"ocid1", "synthetic-message", s.ConfigurationSHA256, testAttemptID} {
		if strings.Contains(logs.String(), private) {
			t.Fatal("notification metadata/request leaked into logs")
		}
	}
}
