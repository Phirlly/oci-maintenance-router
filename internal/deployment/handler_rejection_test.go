package deployment

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandlerRejectsMismatchedOrExtendedRequests(t *testing.T) {
	for _, kind := range []string{"config", "release", "extra", "trailing", "read-cancel", "connect-cancel"} {
		h, s, store := handlerFixture(t)
		ctx, cancel := context.WithCancel(testContext(t))
		defer cancel()
		body := invocation(s)
		switch kind {
		case "config":
			body = strings.Replace(body, s.ConfigurationSHA256, strings.Repeat("0", 64), 1)
		case "release":
			body = strings.Replace(body, s.ReleaseSHA256, strings.Repeat("0", 64), 1)
		case "extra":
			body = strings.TrimSuffix(body, "}") + `,"mode":"LIVE"}`
		case "trailing":
			body += "{}"
		case "connect-cancel":
			h.connect = func(Settings) (Store, error) { cancel(); return store, nil }
		}
		input := &cancelInput{Reader: strings.NewReader(body)}
		if kind == "read-cancel" {
			input.cancel = cancel
		}
		out := httptest.NewRecorder()
		h.Serve(ctx, input, out)
		if !strings.Contains(out.Body.String(), `"status":"not-ready"`) || store.reads != 0 {
			t.Fatalf("unsafe %s: %s", kind, out.Body)
		}
	}
}

type cancelInput struct {
	*strings.Reader
	cancel context.CancelFunc
}

func (r *cancelInput) Read(p []byte) (int, error) {
	n, e := r.Reader.Read(p)
	if r.cancel != nil {
		r.cancel()
	}
	return n, e
}
