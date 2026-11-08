package deployment

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http/httptest"
	"strings"
	"testing"
)

const testAttemptID = "01234567-89ab-cdef-0123-456789abcdef"

func invocation(s Settings) string {
	b, _ := json.Marshal(map[string]string{"operation": "deployment-check", "configurationSha256": s.ConfigurationSHA256, "releaseSha256": s.ReleaseSHA256, "checkAttemptId": testAttemptID})
	return string(b)
}

func handlerFixture(t *testing.T) (*Handler, Settings, *memoryStore) {
	s, store := fixture(t)
	b, _ := json.Marshal(s)
	h := New(func(string) string { return string(b) })
	h.connect = func(Settings) (Store, error) { return store, nil }
	return h, s, store
}

func TestHandlerReturnsBoundedDeploymentResult(t *testing.T) {
	h, s, _ := handlerFixture(t)
	out := httptest.NewRecorder()
	h.Serve(testContext(t), strings.NewReader(invocation(s)), out)
	var got Result
	if json.Unmarshal(out.Body.Bytes(), &got) != nil || got.Status != "deployment-verified" || got.SendingEnabled || out.Body.Len() > 1024 {
		t.Fatalf("unexpected output %s", out.Body)
	}
}

func TestHandlerRejectsUnsupportedOrUnboundedInput(t *testing.T) {
	for _, body := range []string{"", "LIVE", "{}", `{"operation":"LIVE"}`, strings.Repeat("x", 1025)} {
		h, _, store := handlerFixture(t)
		out := httptest.NewRecorder()
		h.Serve(testContext(t), strings.NewReader(body), out)
		if !strings.Contains(out.Body.String(), `"status":"not-ready"`) || store.reads != 0 {
			t.Fatalf("unsafe request: %s", out.Body)
		}
	}
}

func TestHandlerRequiresBudgetBeforeBootstrap(t *testing.T) {
	for _, canceled := range []bool{false, true} {
		h, s, store := handlerFixture(t)
		ctx := context.Background()
		if canceled {
			var cancel context.CancelFunc
			ctx, cancel = context.WithCancel(testContext(t))
			cancel()
		}
		out := httptest.NewRecorder()
		h.Serve(ctx, strings.NewReader(invocation(s)), out)
		if store.reads != 0 || !strings.Contains(out.Body.String(), `"code":"deadline"`) {
			t.Fatalf("unbounded invocation %s", out.Body)
		}
	}
}

func TestHandlerDoesNotExposeReadOrBootstrapErrors(t *testing.T) {
	for _, kind := range []string{"read", "settings", "connect"} {
		h, s, store := handlerFixture(t)
		var input io.Reader = strings.NewReader(invocation(s))
		switch kind {
		case "read":
			input = failedInput{}
		case "settings":
			h.getenv = func(string) string { return "private-value" }
		case "connect":
			h.connect = func(Settings) (Store, error) { return nil, errors.New("private-value") }
		}
		out := httptest.NewRecorder()
		h.Serve(testContext(t), input, out)
		if strings.Contains(out.Body.String(), "private-value") || store.reads != 0 || !strings.Contains(out.Body.String(), `"status":"not-ready"`) {
			t.Fatalf("unsafe failure %s", out.Body)
		}
	}
}

type failedInput struct{}

func (failedInput) Read([]byte) (int, error) { return 0, errors.New("private-value") }
