package ocistorage

import (
	"crypto/rsa"
	"strings"
	"testing"

	"oci-maintenance-router/internal/configuration"
)

type unusedProvider struct{}

func (unusedProvider) PrivateRSAKey() (*rsa.PrivateKey, error) { panic("constructor read credentials") }
func (unusedProvider) KeyID() (string, error)                  { panic("constructor read credentials") }

func TestConstructorUsesDeploymentLocationWithoutReadingCredentials(t *testing.T) {
	c, err := New("us-example-1", configuration.LedgerLocation{Namespace: "old/name +雪", Bucket: "Ledger_1.2"}, unusedProvider{})
	if err != nil || c == nil {
		t.Fatalf("valid location rejected: %v", err)
	}
	if c.endpoint != "https://objectstorage.us-example-1.oraclecloud.com" || c.path != "/n/old%2Fname%20+%E9%9B%AA/b/Ledger_1.2/o/" {
		t.Fatalf("location changed: %s%s", c.endpoint, c.path)
	}
	if c.signer == nil || c.transport == nil {
		t.Fatal("missing authenticated transport")
	}
}

func TestConstructorRejectsUnsafeLocation(t *testing.T) {
	for _, region := range []string{"", "Us-example-1", "us-example-1.evil", "a/", "a:443", "a-", strings.Repeat("a", 64)} {
		c, err := New(region, configuration.LedgerLocation{Namespace: "ns", Bucket: "ledger"}, unusedProvider{})
		if err == nil || c != nil {
			t.Errorf("unsafe region accepted: %q", region)
		}
	}
	for _, namespace := range []string{"", ".", "..", "a\n", "a\x7f", string([]byte{255}), strings.Repeat("n", 1025)} {
		c, err := New("us-example-1", configuration.LedgerLocation{Namespace: namespace, Bucket: "ledger"}, unusedProvider{})
		if err == nil || c != nil {
			t.Errorf("unsafe namespace accepted: %q", namespace)
		}
	}
	for _, bucket := range []string{"", ".", "..", "a/b", "a b", "雪", strings.Repeat("b", 257)} {
		c, err := New("us-example-1", configuration.LedgerLocation{Namespace: "ns", Bucket: bucket}, unusedProvider{})
		if err == nil || c != nil {
			t.Errorf("unsafe bucket accepted: %q", bucket)
		}
	}
	c, err := New("us-example-1", configuration.LedgerLocation{Namespace: "ns", Bucket: "ledger"}, nil)
	if err == nil || c != nil {
		t.Fatal("nil provider accepted")
	}
}
