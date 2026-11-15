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

func TestFDKInputRejectionReportsObservedTransportShape(t *testing.T) {
	h, s, objects, _ := operationalFixture(t)
	configureOperational(t, h, s, objects, func(v map[string]any) { v["mode"] = "PAUSED" })
	var logs bytes.Buffer
	h.logger = log.New(&logs, "", 0)
	client := protocolClient(t, h)
	for _, tc := range []struct {
		name, prefix, category, shape string
		http, metadata, malformed     bool
	}{
		{name: "absent", category: "command_invalid", shape: "absent"},
		{name: "prefixed ordinary", prefix: "Fn-Http-H-", metadata: true, category: "command_invalid", shape: "valid"},
		{name: "unprefixed HTTP", http: true, metadata: true, category: "command_invalid", shape: "absent"},
		{name: "invalid mapped HTTP", prefix: "Fn-Http-H-", http: true, metadata: true, malformed: true, category: "metadata_invalid", shape: "multiple"},
		{name: "valid mapped HTTP", prefix: "Fn-Http-H-", http: true, metadata: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			logs.Reset()
			req, err := http.NewRequest(http.MethodPost, "http://socket/call", strings.NewReader(`{"private":"private-value"}`))
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Fn-Deadline", time.Now().Add(time.Minute).UTC().Format(time.RFC3339Nano))
			if tc.http {
				req.Header.Set("Fn-Intent", "httprequest")
			}
			if tc.metadata {
				for key, values := range intakeHeaders() {
					req.Header[tc.prefix+key] = values
				}
			}
			if tc.malformed {
				req.Header.Add(tc.prefix+"X-OCI-NS-MessageId", "private-duplicate")
			}
			resp, err := client.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			body, readErr := io.ReadAll(io.LimitReader(resp.Body, 4097))
			closeErr := resp.Body.Close()
			var result OperationalResult
			if readErr != nil || closeErr != nil || json.Unmarshal(body, &result) != nil || len(body) > 4096 || resp.StatusCode != http.StatusOK || objects.version != 0 {
				t.Fatal("FDK response or no-effects contract changed", string(body), readErr, closeErr)
			}
			var got map[string]any
			if json.Unmarshal(logs.Bytes(), &got) != nil || bytes.Count(logs.Bytes(), []byte("\n")) != 1 || strings.Contains(logs.String(), "private") || strings.Contains(logs.String(), "ocid1") || bytes.Contains(body, []byte("inputRejection")) {
				t.Fatal("unsafe or missing diagnostic", logs.String())
			}
			if tc.category == "" {
				if result.Status != "idle" || got["inputRejection"] != nil {
					t.Fatal("valid mapped metadata was rejected", string(body), logs.String())
				}
				return
			}
			detail, ok := got["inputRejection"].(map[string]any)
			if !ok || result.Code != "request_invalid" || detail["httpContext"] != tc.http || detail["category"] != tc.category {
				t.Fatal("missing observed transport diagnostics", logs.String())
			}
			if detail["prefixed"].(map[string]any)["messageId"] != tc.shape {
				t.Fatal("wrong FDK-prefixed shape", detail)
			}
			wantDirect := "absent"
			if tc.malformed {
				wantDirect = "multiple"
			}
			if detail["direct"].(map[string]any)["messageId"] != wantDirect {
				t.Fatal("wrong FDK-direct shape", detail)
			}
		})
	}
}
