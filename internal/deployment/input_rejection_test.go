package deployment

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

// Observe only the public response and structured log, not diagnostic internals.
func rejectedInputLog(t *testing.T, headers http.Header, input io.Reader) map[string]any {
	t.Helper()
	h := New(func(string) string { t.Fatal("rejected input reached bootstrap"); return "" })
	var logs bytes.Buffer
	h.logger = log.New(&logs, "", 0)
	out := httptest.NewRecorder()
	h.Serve(withInvocationHeaders(operationalContext(t), headers), input, out)
	var result Result
	if json.Unmarshal(out.Body.Bytes(), &result) != nil || result != failed("request_invalid") || strings.Contains(out.Body.String(), "inputRejection") {
		t.Fatalf("diagnostics changed response: %s", out.Body)
	}
	var got map[string]any
	if json.Unmarshal(logs.Bytes(), &got) != nil || bytes.Count(logs.Bytes(), []byte("\n")) != 1 || got["code"] != "request_invalid" {
		t.Fatal("missing single rejection log", logs.String())
	}
	for _, private := range []string{"private", "ocid1", "synthetic-message", "synthetic-type", "https://"} {
		if strings.Contains(logs.String(), private) {
			t.Fatal("diagnostic leaked input", private)
		}
	}
	detail, ok := got["inputRejection"].(map[string]any)
	if !ok || len(detail) != 6 || detail["httpContext"] != false || logs.Len() > 1024 {
		t.Fatal("missing or unbounded input diagnostic", logs.String())
	}
	return detail
}

func TestRejectedInputLogDistinguishesParserFailures(t *testing.T) {
	for _, tc := range []struct {
		name, category string
		headers        http.Header
		input          io.Reader
	}{
		{"metadata", "metadata_invalid", http.Header{"X-OCI-NS-Private": {"private-value"}}, strings.NewReader(`{"operation":"reconcile"}`)},
		{"notification read", "notification_body_read", intakeHeaders(), failedInput{}},
		{"notification size", "notification_body_size", intakeHeaders(), strings.NewReader(strings.Repeat("x", 65537))},
		{"command read", "command_body_read", nil, failedInput{}},
		{"command size", "command_body_size", nil, strings.NewReader(strings.Repeat("x", 8193))},
		{"command syntax", "command_invalid", nil, strings.NewReader(`{"private":"private-value"}`)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := rejectedInputLog(t, tc.headers, tc.input)
			if got["category"] != tc.category {
				t.Fatalf("wrong failure category: %v", got)
			}
		})
	}
}

func TestInputDiagnosticsDoNotChangeReadsOrSuccessfulLogs(t *testing.T) {
	for _, limit := range []int{8193, 65537} {
		t.Run(strconv.Itoa(limit), func(t *testing.T) {
			input := strings.NewReader(strings.Repeat("x", limit+100))
			var headers http.Header
			if limit == 65537 {
				headers = intakeHeaders()
			}
			rejectedInputLog(t, headers, input)
			if input.Len() != 100 {
				t.Fatal("diagnostics read beyond existing bound")
			}
		})
	}
	for _, kind := range []string{"accepted", "bootstrap", "deadline"} {
		t.Run(kind, func(t *testing.T) {
			h, s, _ := handlerFixture(t)
			ctx := testContext(t)
			if kind == "bootstrap" {
				h.getenv = func(string) string { return "private-invalid" }
			}
			if kind == "deadline" {
				ctx = context.Background()
			}
			var logs bytes.Buffer
			h.logger = log.New(&logs, "", 0)
			h.Serve(ctx, strings.NewReader(invocation(s)), httptest.NewRecorder())
			if strings.Contains(logs.String(), "inputRejection") {
				t.Fatal("diagnostic added outside parser rejection", logs.String())
			}
		})
	}
}
