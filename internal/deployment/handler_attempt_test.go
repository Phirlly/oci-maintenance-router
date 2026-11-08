package deployment

import (
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
)

func attemptResponse(t *testing.T, h *Handler, s Settings, attempt string) (Result, string) {
	t.Helper()
	body := strings.Replace(invocation(s), testAttemptID, attempt, 1)
	out := httptest.NewRecorder()
	h.Serve(testContext(t), strings.NewReader(body), out)
	var result Result
	if err := json.Unmarshal(out.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if out.Body.Len() > 1024 || result.SendingEnabled {
		t.Fatalf("unsafe response %s", out.Body)
	}
	return result, result.CheckAttemptID
}

func TestHandlerEchoesValidAttemptOnSuccessAndFailure(t *testing.T) {
	for _, tc := range []struct{ kind, code string }{
		{"success", ""}, {"bootstrap", "bootstrap_invalid"},
		{"mismatch", "request_mismatch"}, {"authentication", "authentication"},
		{"storage", "catalog_read"},
	} {
		t.Run(tc.kind, func(t *testing.T) {
			h, s, store := handlerFixture(t)
			switch tc.kind {
			case "bootstrap":
				h.getenv = func(string) string { return "invalid" }
			case "mismatch":
				s.ReleaseSHA256 = strings.Repeat("0", 64)
			case "authentication":
				h.connect = func(Settings) (Store, error) { return nil, errors.New("private-error") }
			case "storage":
				store.readError = errors.New("private-error")
			}
			// Terraform requires UUID shape, not RFC version/variant bits.
			got, attempt := attemptResponse(t, h, s, testAttemptID)
			if attempt != testAttemptID || got.Code != tc.code || (got.Status == "deployment-verified") != (tc.kind == "success") {
				t.Fatalf("result=%+v attempt=%q", got, attempt)
			}
		})
	}
}

func TestHandlerRejectsInvalidAttemptBeforeBootstrap(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value any
	}{
		{"missing", nil}, {"null", nil}, {"empty", ""}, {"number", 42},
		{"uppercase", strings.ToUpper(testAttemptID)},
		{"short", testAttemptID[:35]}, {"long", testAttemptID + "0"},
		{"separators", strings.ReplaceAll(testAttemptID, "-", "_")},
		{"nonhex", "g" + testAttemptID[1:]}, {"whitespace", " " + testAttemptID},
		{"newline", testAttemptID + "\n"}, {"oversized", strings.Repeat("x", 1025)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h, s, store := handlerFixture(t)
			var request map[string]any
			if err := json.Unmarshal([]byte(invocation(s)), &request); err != nil {
				t.Fatal(err)
			}
			request["checkAttemptId"] = tc.value
			if tc.name == "missing" {
				delete(request, "checkAttemptId")
			}
			body, err := json.Marshal(request)
			if err != nil {
				t.Fatal(err)
			}
			bootstrap, connects := 0, 0
			h.getenv = func(string) string { bootstrap++; return "invalid" }
			h.connect = func(Settings) (Store, error) { connects++; return store, nil }
			out := httptest.NewRecorder()
			h.Serve(testContext(t), strings.NewReader(string(body)), out)
			if out.Body.String() != "{\"status\":\"not-ready\",\"code\":\"request_invalid\",\"sendingEnabled\":false}\n" || bootstrap != 0 || connects != 0 || store.reads != 0 || store.creates != 0 {
				t.Fatalf("unsafe rejection %s bootstrap=%d connects=%d", out.Body, bootstrap, connects)
			}
		})
	}
}
