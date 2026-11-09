package ocisecrets

import (
	"crypto/rsa"
	"strings"
	"testing"
)

type unusedProvider struct{}

func (unusedProvider) PrivateRSAKey() (*rsa.PrivateKey, error) { panic("constructor read credentials") }
func (unusedProvider) KeyID() (string, error)                  { panic("constructor read credentials") }

func TestConstructorFixesRegionalEndpointWithoutReadingCredentials(t *testing.T) {
	c, err := New("us-example-1", "exm", unusedProvider{})
	if err != nil || c == nil {
		t.Fatalf("valid deployment rejected: %v", err)
	}
	if c.endpoint != "https://secrets.vaults.us-example-1.oci.oraclecloud.com" || c.region != "us-example-1" || c.regionKey != "exm" || c.signer == nil || c.transport == nil {
		t.Fatal("missing fixed endpoint, scope or authenticated transport")
	}
}

func TestConstructorRejectsUnsafeRegionAndKey(t *testing.T) {
	for _, region := range []string{"", "Us-example-1", "us-example-1.evil", "a/", "a:443", "a-", strings.Repeat("a", 64)} {
		if c, err := New(region, "exm", unusedProvider{}); c != nil || err == nil {
			t.Errorf("unsafe region accepted: %q", region)
		}
	}
	for _, key := range []string{"", "EXM", "a-b", "a/", "a.b", strings.Repeat("a", 64)} {
		if c, err := New("us-example-1", key, unusedProvider{}); c != nil || err == nil {
			t.Errorf("unsafe region key accepted: %q", key)
		}
	}
	if c, err := New("us-example-1", "exm", nil); c != nil || err == nil {
		t.Fatal("nil provider accepted")
	}
}
