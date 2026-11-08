package ocistorage

import (
	"bytes"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestUnusableReadResponsesReturnNoState(t *testing.T) {
	key := generatedKey(t)
	cases := []struct {
		name    string
		status  int
		headers http.Header
		body    string
		code    Code
	}{
		{"missing_tag", 200, nil, "{}", InvalidResponse},
		{"duplicate_tag", 200, http.Header{"Etag": {"one", "two"}}, "{}", InvalidResponse},
		{"wildcard_tag", 200, http.Header{"Etag": {"*"}}, "{}", InvalidResponse},
		{"weak_tag", 200, http.Header{"Etag": {"W/\"one\""}}, "{}", InvalidResponse},
		{"range", 200, http.Header{"Etag": {"one"}, "Content-Range": {"bytes 0-1/10"}}, "{}", InvalidResponse},
		{"encoding", 200, http.Header{"Etag": {"one"}, "Content-Encoding": {"gzip"}}, "{}", InvalidResponse},
		{"partial", 206, http.Header{"Etag": {"one"}}, "{}", Service},
		{"not_found_or_denied", 404, nil, "synthetic-private-response", Service},
		{"unexpected_success", 201, http.Header{"Etag": {"one"}}, "{}", Service},
		{"truncated", 200, http.Header{"Etag": {"one"}, "Content-Length": {"100"}}, "{}", InvalidResponse},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := tlsClient(t, syntheticKey{key: key}, func(w http.ResponseWriter, r *http.Request) {
				for k, v := range tt.headers {
					w.Header()[k] = v
				}
				w.WriteHeader(tt.status)
				_, _ = io.WriteString(w, tt.body)
			})
			got, err := c.Get(budget(t), "state")
			wantError(t, err, tt.code, tt.status, false)
			if got.Body != nil || got.ETag != "" || strings.Contains(err.Error(), tt.body) {
				t.Fatal("unsafe read result")
			}
		})
	}
}

func TestUnusableWriteResponsesAreUncertain(t *testing.T) {
	key := generatedKey(t)
	for _, tt := range []struct {
		name    string
		headers http.Header
		body    string
	}{
		{"missing", nil, ""}, {"duplicate", http.Header{"Etag": {"one", "two"}}, ""},
		{"list", http.Header{"Etag": {"one,two"}}, ""}, {"body", http.Header{"Etag": {"one"}}, "unexpected"},
		{"truncated", http.Header{"Etag": {"one"}, "Content-Length": {"100"}}, "x"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := tlsClient(t, syntheticKey{key: key}, func(w http.ResponseWriter, r *http.Request) {
				for k, v := range tt.headers {
					w.Header()[k] = v
				}
				_, _ = io.WriteString(w, tt.body)
			})
			tag, err := c.Create(budget(t), "state", nil)
			wantError(t, err, InvalidResponse, 200, true)
			if tag != "" {
				t.Fatal("unconfirmed write returned ETag")
			}
		})
	}
}

func TestExactObjectByteLimitAndReadOwnership(t *testing.T) {
	payload := bytes.Repeat([]byte{0xff}, 1<<20)
	c, _ := tlsClient(t, syntheticKey{key: generatedKey(t)}, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "PUT" {
			body, _ := io.ReadAll(r.Body)
			if !bytes.Equal(body, payload) {
				t.Error("upload boundary changed")
			}
		}
		w.Header().Set("ETag", "opaque-tag")
		if r.Method == "GET" {
			_, _ = w.Write(payload)
		}
	})
	if _, err := c.Create(budget(t), "state", payload); err != nil {
		t.Fatal(err)
	}
	obj, err := c.Get(budget(t), "state")
	if err != nil || !bytes.Equal(obj.Body, payload) {
		t.Fatalf("exact cap failed: %v", err)
	}
	obj.Body[0] = 0
	next, err := c.Get(budget(t), "state")
	if err != nil || !bytes.Equal(next.Body, payload) {
		t.Fatal("read buffer aliases retained state")
	}
}

func TestResponseLimitsFailClosed(t *testing.T) {
	key := generatedKey(t)
	for _, kind := range []string{"declared_body", "streamed_body", "headers", "put_body"} {
		t.Run(kind, func(t *testing.T) {
			c, _ := tlsClient(t, syntheticKey{key: key}, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("ETag", "tag")
				switch kind {
				case "headers":
					w.Header().Set("X-Oversized", strings.Repeat("h", 32<<10))
				case "declared_body":
					w.Header().Set("Content-Length", "1048577")
				case "streamed_body":
					w.WriteHeader(200)
					w.(http.Flusher).Flush()
				}
				_, _ = io.WriteString(w, strings.Repeat("b", (1<<20)+1))
			})
			var err error
			if kind == "put_body" {
				_, err = c.Create(budget(t), "state", nil)
			} else {
				_, err = c.Get(budget(t), "state")
			}
			code, status := TooLarge, 200
			if kind == "headers" {
				code, status = Transport, 0
			}
			wantError(t, err, code, status, kind == "put_body")
		})
	}
}
