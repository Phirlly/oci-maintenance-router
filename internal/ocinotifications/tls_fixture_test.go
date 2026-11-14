package ocinotifications

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/oracle/oci-go-sdk/v65/common"
)

const tenancy = "ocid1.tenancy.oc1..example"
const compartment = "ocid1.compartment.oc1..example"
const topicID = "ocid1.onstopic.oc1.iad.example"
const subscriptionID = "ocid1.onssubscription.oc1.iad.example"
const controlOrigin = "https://notification.us-ashburn-1.oraclecloud.com"
const dataOrigin = "https://cell1.notification.us-ashburn-1.oci.oraclecloud.com"
const topicJSON = `{"topicId":"` + topicID + `","compartmentId":"` + compartment + `","lifecycleState":"ACTIVE","apiEndpoint":"` + dataOrigin + `"}`
const subscriptionJSON = `{"id":"` + subscriptionID + `","topicId":"` + topicID + `","compartmentId":"` + tenancy + `","lifecycleState":"ACTIVE","protocol":"EMAIL","endpoint":"Team@example.invalid"}`

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

type unusedProvider struct{}

func (*unusedProvider) PrivateRSAKey() (*rsa.PrivateKey, error) { panic("unexpected credentials") }
func (*unusedProvider) KeyID() (string, error)                  { panic("unexpected credentials") }

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
	key := generatedKey(t)
	hosts := []string{strings.TrimPrefix(controlOrigin, "https://"), strings.TrimPrefix(dataOrigin, "https://")}
	cert := &x509.Certificate{SerialNumber: big.NewInt(1), DNSNames: hosts, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, IsCA: true, BasicConstraintsValid: true}
	der, err := x509.CreateCertificate(rand.Reader, cert, cert, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewUnstartedServer(handler)
	server.EnableHTTP2 = true
	server.TLS = &tls.Config{Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}}}
	server.StartTLS()
	t.Cleanup(server.Close)
	c, err := New("us-ashburn-1", "iad", tenancy, provider)
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	pool.AddCert(server.Certificate())
	c.transport.TLSClientConfig.RootCAs = pool
	address := server.Listener.Addr().String()
	c.transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		if addr != hosts[0]+":443" && addr != hosts[1]+":443" {
			t.Error("unexpected external dial")
			return nil, errors.New("unexpected dial")
		}
		return (&net.Dialer{}).DialContext(ctx, network, address)
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
func serveChannel(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/20181201/topics/") {
		io.WriteString(w, topicJSON)
	} else {
		io.WriteString(w, subscriptionJSON)
	}
}
