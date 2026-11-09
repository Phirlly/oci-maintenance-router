package pagerduty

import (
	"io"
	"net/http"
	"testing"
)

func TestAcceptedMalformedBodyAndMissingCloseHeader(t *testing.T) {
	for _, framing := range []string{"Content-Length: 10000\r\n", "Transfer-Encoding: chunked\r\n"} {
		stopped := make(chan struct{})
		c, _ := tlsClient(t, func(w http.ResponseWriter, r *http.Request) {
			_, _ = io.Copy(io.Discard, r.Body)
			conn, buffer, err := w.(http.Hijacker).Hijack()
			if err != nil {
				t.Error(err)
				close(stopped)
				return
			}
			defer conn.Close()
			// Deliberately omit Connection: close, disregard request close preference,
			// send malformed/truncated content, then wait for the client to disconnect.
			_, _ = buffer.WriteString("HTTP/1.1 202 Accepted\r\n" + framing + "\r\nnot JSON or a valid chunk")
			_ = buffer.Flush()
			_, _ = io.Copy(io.Discard, conn)
			close(stopped)
		})
		got, err := c.Send(budget(t), []byte(savedEvent), []byte(syntheticKey))
		if err != nil || got.Status != 202 {
			t.Fatalf("observed acceptance lost: %+v %v", got, err)
		}
		await(t, stopped)
	}
}
