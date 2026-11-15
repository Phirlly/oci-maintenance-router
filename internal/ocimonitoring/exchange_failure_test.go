package ocimonitoring

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type failedKey struct{}

func (failedKey) PrivateRSAKey() (*rsa.PrivateKey, error) {
	return nil, errors.New("private-key-detail")
}
func (failedKey) KeyID() (string, error) { return "synthetic-only", nil }

func TestAuthenticationFailureDoesNotDispatchOrExposeProviderData(t *testing.T) {
	calls := 0
	c, _ := tlsClient(t, failedKey{}, func(http.ResponseWriter, *http.Request) { calls++ })
	err := c.Publish(budget(t), Observation{At: time.Now(), CompletedScan: true})
	if err == nil || err.Error() != "operational metrics: authentication" || calls != 0 {
		t.Fatalf("authentication %v calls=%d", err, calls)
	}
}

func TestUncertainResponseNeverReplaysPublication(t *testing.T) {
	calls := 0
	c, _ := tlsClient(t, syntheticKey{key: generatedKey(t)}, func(w http.ResponseWriter, r *http.Request) {
		calls++
		_, _ = io.ReadAll(r.Body)
		conn, _, err := w.(http.Hijacker).Hijack()
		if err != nil {
			t.Error(err)
			return
		}
		_ = conn.Close()
	})
	if err := c.Publish(budget(t), Observation{At: time.Now(), CompletedScan: true}); err == nil || calls != 1 {
		t.Fatalf("uncertain exchange %v calls=%d", err, calls)
	}
}

func TestUntrustedTLSCertificatePreventsMetricDispatch(t *testing.T) {
	calls := 0
	c, _ := tlsClient(t, syntheticKey{key: generatedKey(t)}, func(http.ResponseWriter, *http.Request) { calls++ })
	c.transport.TLSClientConfig.RootCAs = x509.NewCertPool()
	err := c.Publish(budget(t), Observation{At: time.Now(), CompletedScan: true})
	if err == nil || err.Error() != "operational metrics: transport" || calls != 0 {
		t.Fatalf("untrusted endpoint received metrics: %v calls=%d", err, calls)
	}
}

func TestResponseSizeBoundIncludesChunkedBodies(t *testing.T) {
	c, _ := tlsClient(t, syntheticKey{key: generatedKey(t)}, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.(http.Flusher).Flush()
		io.WriteString(w, strings.Repeat(" ", bodyLimit+1)+`{"failedMetricsCount":0}`)
	})
	if err := c.Publish(budget(t), Observation{At: time.Now(), CompletedScan: true}); err == nil {
		t.Fatal("unbounded body accepted")
	}
}

type failingBody struct {
	io.Reader
	closeFailure bool
}

func (b failingBody) Close() error {
	if b.closeFailure {
		return errors.New("private-close")
	}
	return nil
}

type failedReader struct{}

func (failedReader) Read([]byte) (int, error) { return 0, errors.New("private-read") }

func TestBodyReadAndCloseFailuresCannotBecomeAcceptance(t *testing.T) {
	for _, body := range []io.ReadCloser{failingBody{Reader: failedReader{}}, failingBody{Reader: strings.NewReader(`{"failedMetricsCount":0}`), closeFailure: true}} {
		err := acceptedResponse(budget(t), &http.Response{StatusCode: 200, Header: http.Header{}, ContentLength: -1, Body: body})
		if err == nil || strings.Contains(err.Error(), "private") {
			t.Fatal("invalid body acceptance", err)
		}
	}
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	err := acceptedResponse(ctx, &http.Response{StatusCode: 200, Header: http.Header{}, ContentLength: -1, Body: io.NopCloser(strings.NewReader(`{"failedMetricsCount":0}`))})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("late acceptance", err)
	}
}
