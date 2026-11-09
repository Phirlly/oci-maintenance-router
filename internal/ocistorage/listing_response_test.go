package ocistorage

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

func TestListingResponseFailuresReturnNoPage(t *testing.T) {
	key := generatedKey(t)
	for _, kind := range []string{"encoding", "range", "declared size", "streamed size", "truncated", "malformed", "large headers", "wrong schema"} {
		t.Run(kind, func(t *testing.T) {
			c, _ := tlsClient(t, syntheticKey{key: key}, func(w http.ResponseWriter, r *http.Request) {
				switch kind {
				case "encoding":
					w.Header().Set("Content-Encoding", "gzip")
				case "range":
					w.Header().Set("Content-Range", "bytes 0-1/2")
				case "declared size":
					w.Header().Set("Content-Length", fmt.Sprint(objectLimit+1))
				case "truncated":
					w.Header().Set("Content-Length", "999")
				case "large headers":
					w.Header().Set("X-Synthetic", strings.Repeat("x", headerLimit+1))
				}
				if kind == "streamed size" {
					w.WriteHeader(200)
					w.(http.Flusher).Flush()
					io.WriteString(w, strings.Repeat(" ", objectLimit+1))
					return
				}
				if kind == "malformed" {
					io.WriteString(w, "{")
					return
				}
				if kind == "wrong schema" {
					io.WriteString(w, `{"objects":null}`)
					return
				}
				io.WriteString(w, `{"objects":[]}`)
			})
			code, status := InvalidResponse, 200
			if kind == "declared size" || kind == "streamed size" {
				code = TooLarge
			}
			if kind == "large headers" {
				code = Transport
				status = 0
			}
			m, err := noticeStore(t, c, noticeState(t)).ListPage(budget(t), "")
			wantError(t, err, code, status, false)
			emptyPage(t, m)
		})
	}
}

type listingResponseBody struct {
	io.Reader
	close func() error
}

func (b listingResponseBody) Close() error { return b.close() }

type listingBrokenReader struct{}

func (listingBrokenReader) Read([]byte) (int, error) {
	return 0, errors.New("synthetic-private-response")
}

func TestListingReadAndCloseFailuresDiscardBytes(t *testing.T) {
	for _, phase := range []string{"read", "close", "canceled close"} {
		ctx, cancel := context.WithCancel(budget(t))
		defer cancel()
		body := listingResponseBody{Reader: bytes.NewBufferString(`{"objects":[]}`), close: func() error { return errors.New("synthetic-private-close") }}
		code := InvalidResponse
		if phase == "read" {
			body.Reader = listingBrokenReader{}
		}
		if phase == "canceled close" {
			body.close = func() error { cancel(); return nil }
			code = Canceled
		}
		data, err := readListingBody(ctx, &http.Response{StatusCode: 200, Body: body})
		wantError(t, err, code, 200, false)
		if data != nil {
			t.Fatal("partial bytes escaped")
		}
	}
}
