package ocistorage

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestHistoryUnusableResponsesNeverEstablishAbsence(t *testing.T) {
	key := generatedKey(t)
	for _, kind := range []string{"encoding", "range", "declared size", "streamed size", "truncated", "malformed", "large headers", "wrong schema", "item limit"} {
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
				body := `{"items":[]}`
				switch kind {
				case "streamed size":
					w.WriteHeader(200)
					w.(http.Flusher).Flush()
					body = strings.Repeat(" ", objectLimit+1)
				case "malformed":
					body = "{"
				case "wrong schema":
					body = `{"items":null}`
				case "item limit":
					body = `{"items":[{},{}]}`
				}
				_, _ = io.WriteString(w, body)
			})
			code, status := InvalidResponse, 200
			if kind == "declared size" || kind == "streamed size" || kind == "item limit" {
				code = TooLarge
			}
			if kind == "large headers" {
				code, status = Transport, 0
			}
			empty, err := c.HistoryIsEmpty(budget(t), "control/head.json")
			wantError(t, err, code, status, false)
			if empty {
				t.Fatal("unusable response established absence")
			}
		})
	}
}
