package ocisecrets

import (
	"bytes"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
)

func TestHTTPResponseBoundsAndIntegrity(t *testing.T) {
	key := generatedKey(t)
	body := bundleBody(t, testSecretID, 7, []byte("synthetic-key"))
	for _, tc := range []struct {
		name          string
		header, value string
		body          []byte
		flush         bool
		code          Code
		status        int
	}{
		{"declared overflow", "Content-Length", strconv.Itoa(responseLimit + 1), nil, false, TooLarge, 200},
		{"streamed overflow", "", "", bytes.Repeat([]byte("x"), responseLimit+1), true, TooLarge, 200},
		{"truncated body", "Content-Length", strconv.Itoa(len(body) + 1), body, false, InvalidResponse, 200},
		{"partial", "Content-Range", "bytes 0-100/200", body, false, InvalidResponse, 200},
		{"encoded", "Content-Encoding", "gzip", body, false, InvalidResponse, 200},
		{"large header", "X-Metadata", strings.Repeat("x", headerLimit+1), body, false, Transport, 0},
		{"wrong identity", "", "", bundleBody(t, "other", 7, []byte("synthetic-key")), false, InvalidResponse, 200},
		{"wrong version", "", "", bundleBody(t, testSecretID, 8, []byte("synthetic-key")), false, InvalidResponse, 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := tlsClient(t, syntheticKey{key: key}, func(w http.ResponseWriter, r *http.Request) {
				if tc.header != "" {
					w.Header().Set(tc.header, tc.value)
				}
				if tc.flush {
					w.(http.Flusher).Flush()
				}
				_, _ = w.Write(tc.body)
			})
			b, err := c.GetVersion(budget(t), testSecretID, 7)
			wantError(t, err, tc.code, tc.status)
			wantEmpty(t, b)
		})
	}
}

func TestExactWireBudgetAllowsOptionalMetadata(t *testing.T) {
	body := bundleBody(t, testSecretID, 7, []byte("synthetic-key"))
	metadata := `,"metadata":"` + strings.Repeat("x", responseLimit-len(body)-14) + `"}`
	body = append(body[:len(body)-1], metadata...)
	if len(body) != responseLimit {
		t.Fatalf("fixture size=%d", len(body))
	}
	c, _ := tlsClient(t, syntheticKey{key: generatedKey(t)}, func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(w, bytes.NewReader(body))
	})
	b, err := c.GetCurrent(budget(t), testSecretID)
	if err != nil || b.Version() != 7 || string(b.Bytes()) != "synthetic-key" {
		t.Fatalf("exact wire budget rejected: %v", err)
	}
}
