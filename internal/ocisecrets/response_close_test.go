package ocisecrets

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"testing"
)

type responseBody struct {
	io.Reader
	close func() error
}

func (b responseBody) Close() error { return b.close() }

type brokenReader struct{}

func (brokenReader) Read([]byte) (int, error) { return 0, errors.New("synthetic-private-response") }

func TestResponseReadAndCloseFailuresReturnNoSecret(t *testing.T) {
	for _, phase := range []string{"read", "close", "canceled close"} {
		t.Run(phase, func(t *testing.T) {
			ctx, cancel := context.WithCancel(budget(t))
			defer cancel()
			body := responseBody{Reader: bytes.NewReader(bundleBody(t, testSecretID, 7, []byte("synthetic-key")))}
			body.close = func() error { return errors.New("synthetic-private-close") }
			code := InvalidResponse
			if phase == "read" {
				body.Reader = brokenReader{}
			}
			if phase == "canceled close" {
				body.close = func() error { cancel(); return nil }
				code = Canceled
			}
			b, err := readBundle(ctx, &http.Response{StatusCode: 200, Body: body}, testSecretID, 7)
			wantError(t, err, code, 200)
			wantEmpty(t, b)
			if err.Error() != "secret retrieval: "+string(code) {
				t.Fatal("response error escaped")
			}
		})
	}
}
