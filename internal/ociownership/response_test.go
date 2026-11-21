package ociownership

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestResponseFailuresReturnNoMetadata(t *testing.T) {
	key := generatedKey(t)
	for _, kind := range []string{"encoding", "range", "declared size", "streamed size", "truncated", "malformed", "large headers", "wrong identity"} {
		t.Run(kind, func(t *testing.T) {
			c, _ := tlsClient(t, syntheticKey{key: key}, func(w http.ResponseWriter, r *http.Request) {
				switch kind {
				case "encoding":
					w.Header().Set("Content-Encoding", "gzip")
				case "range":
					w.Header().Set("Content-Range", "bytes 0-1/2")
				case "declared size":
					w.Header().Set("Content-Length", fmt.Sprint(bodyLimit+1))
				case "truncated":
					w.Header().Set("Content-Length", "999")
				case "large headers":
					w.Header().Set("X-Synthetic", strings.Repeat("x", headerLimit+1))
				}
				if kind == "streamed size" {
					w.WriteHeader(200)
					w.(http.Flusher).Flush()
					io.WriteString(w, strings.Repeat(" ", bodyLimit+1))
					return
				}
				if kind == "malformed" {
					io.WriteString(w, "{")
					return
				}
				if kind == "wrong identity" {
					io.WriteString(w, strings.Replace(minimal, dbID, dbID+"other", 1))
					return
				}
				io.WriteString(w, minimal)
			})
			code, status := InvalidResponse, 200
			if kind == "declared size" || kind == "streamed size" {
				code = TooLarge
			}
			if kind == "large headers" {
				code = Transport
				status = 0
			}
			m, err := c.Get(budget(t), dbID)
			wantError(t, err, code, status)
			wantEmpty(t, m)
		})
	}
}

type responseBody struct {
	io.Reader
	close func() error
}

func (b responseBody) Close() error { return b.close() }

type brokenReader struct{}

func (brokenReader) Read([]byte) (int, error) { return 0, errors.New("synthetic-private-response") }

func TestReadAndCloseFailuresDiscardBytes(t *testing.T) {
	for _, phase := range []string{"read", "close", "canceled close"} {
		ctx, cancel := context.WithCancel(budget(t))
		defer cancel()
		body := responseBody{Reader: bytes.NewBufferString(minimal), close: func() error { return errors.New("synthetic-private-close") }}
		code := InvalidResponse
		if phase == "read" {
			body.Reader = brokenReader{}
		}
		if phase == "canceled close" {
			body.close = func() error { cancel(); return nil }
			code = Canceled
		}
		data, err := readBody(ctx, &http.Response{StatusCode: 200, Body: body})
		wantError(t, err, code, 200)
		if data != nil {
			t.Fatal("partial bytes escaped")
		}
	}
}
