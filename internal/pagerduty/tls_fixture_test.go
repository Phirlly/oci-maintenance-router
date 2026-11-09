package pagerduty

import (
	"context"
	"crypto/x509"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func tlsClient(t *testing.T, handler http.HandlerFunc) (*Client, *httptest.Server) {
	t.Helper()
	server := httptest.NewUnstartedServer(handler)
	server.EnableHTTP2 = true
	server.StartTLS()
	t.Cleanup(server.Close)
	c, err := New("US")
	if err != nil {
		t.Fatal(err)
	}
	// Private loopback wiring; production exposes no endpoint/TLS overrides.
	c.endpoint = server.URL + "/v2/enqueue"
	pool := x509.NewCertPool()
	pool.AddCert(server.Certificate())
	c.transport.TLSClientConfig.RootCAs = pool
	address := server.Listener.Addr().String()
	c.transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		if addr != address {
			t.Error("unexpected external dial")
			return nil, errors.New("unexpected dial")
		}
		return (&net.Dialer{}).DialContext(ctx, network, addr)
	}
	return c, server
}
func budget(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	t.Cleanup(cancel)
	return ctx
}
func await(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(3 * time.Second):
		t.Fatal("boundary not reached")
	}
}
func wantError(t *testing.T, result Response, err error, code Code, sent bool) {
	t.Helper()
	var e *Error
	if result != (Response{}) || !errors.As(err, &e) || e.Code != code || e.MayHaveSent != sent {
		t.Fatalf("unexpected outcome: %+v %v", result, err)
	}
}

type outcome struct {
	response Response
	err      error
}

func receive(t *testing.T, ch <-chan outcome) outcome {
	t.Helper()
	select {
	case got := <-ch:
		return got
	case <-time.After(3 * time.Second):
		t.Fatal("request did not stop")
		return outcome{}
	}
}
