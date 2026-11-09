package ocisecrets

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/oracle/oci-go-sdk/v65/common"
)

type syntheticKey struct {
	key       *rsa.PrivateKey
	beforeKey func()
}

func (p syntheticKey) PrivateRSAKey() (*rsa.PrivateKey, error) {
	if p.beforeKey != nil {
		p.beforeKey()
	}
	return p.key, nil
}
func (p syntheticKey) KeyID() (string, error) { return "synthetic-only", nil }

func generatedKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	return key
}

func tlsClient(t *testing.T, provider common.KeyProvider, handler http.HandlerFunc) (*Client, *httptest.Server) {
	t.Helper()
	server := httptest.NewUnstartedServer(handler)
	server.EnableHTTP2 = true
	server.StartTLS()
	t.Cleanup(server.Close)
	c, err := New("us-example-1", "exm", provider)
	if err != nil {
		t.Fatal(err)
	}
	// Private test wiring only. No endpoint/TLS override is exposed to callers.
	c.endpoint = server.URL
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
