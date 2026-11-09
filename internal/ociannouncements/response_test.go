package ociannouncements

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

func TestHTTPResponseBoundariesReturnNoNoticeOrPage(t *testing.T) {
	key := generatedKey(t)
	for _, kind := range []string{"encoding", "range", "declared size", "streamed size", "truncated", "malformed", "large headers", "duplicate token"} {
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
				case "duplicate token":
					w.Header().Add("opc-next-page", "a")
					w.Header().Add("opc-next-page", "b")
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
				if strings.HasSuffix(r.URL.Path, "/"+noticeID) {
					io.WriteString(w, minimal)
				} else {
					io.WriteString(w, `{"items":[]}`)
				}
			})
			code, status := InvalidResponse, 200
			if kind == "declared size" || kind == "streamed size" {
				code = TooLarge
			}
			if kind == "large headers" {
				code = Transport
				status = 0
			}
			if kind != "duplicate token" {
				n, err := c.Get(budget(t), noticeID)
				wantError(t, err, code, status)
				wantEmpty(t, n)
			}
			p, err := c.ListPage(budget(t), "")
			wantError(t, err, code, status)
			if len(p.IDs) != 0 || p.NextPage != "" {
				t.Fatal("partial page escaped")
			}
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

func TestResponseReadAndCloseFailuresDiscardBytes(t *testing.T) {
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
			t.Fatal("partial body escaped")
		}
	}
}

func TestCancellationAfterDecodeDiscardsResult(t *testing.T) {
	ctx, cancel := context.WithCancel(budget(t))
	cancel()
	wantError(t, decodedError(ctx, nil), Canceled, 200)
	wantError(t, decodedError(budget(t), errors.New("synthetic-private-source")), InvalidResponse, 200)
}
