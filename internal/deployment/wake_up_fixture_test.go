package deployment

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	fdk "github.com/fnproject/fdk-go"
)

// Only Header is used by the deployment entrypoint; the actual FDK supplies the
// complete context in protocol tests and production.
type invocationMetadata struct {
	fdk.Context
	headers http.Header
}

func (m invocationMetadata) Header() http.Header { return m.headers }

func withInvocationHeaders(ctx context.Context, headers http.Header) context.Context {
	return fdk.WithContext(ctx, invocationMetadata{headers: headers})
}

func intakeHeaders() http.Header {
	h := http.Header{}
	h.Set("X-OCI-NS-TopicOcid", "ocid1.onstopic.oc1.iad.intake")
	h.Set("X-OCI-NS-MessageId", "synthetic-message")
	h.Set("X-OCI-NS-MessageType", "synthetic-type")
	return h
}

func functionNotificationHeaders() http.Header {
	return http.Header{"X-OCI-NS-MessageId": {"synthetic-message"}}
}

func automaticResponse(t *testing.T, h *Handler, ctx context.Context, headers http.Header, body io.Reader) OperationalResult {
	t.Helper()
	out := httptest.NewRecorder()
	h.Serve(withInvocationHeaders(ctx, headers), body, out)
	var got OperationalResult
	if err := json.Unmarshal(out.Body.Bytes(), &got); err != nil || out.Body.Len() > 4096 {
		t.Fatalf("unbounded/invalid response %s: %v", out.Body, err)
	}
	return got
}
