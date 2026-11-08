package deployment

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/common/auth"
)

func TestBootstrapRequiresCompleteBoundedMetadata(t *testing.T) {
	s, _ := fixture(t)
	valid, _ := json.Marshal(s)
	for _, raw := range []string{"", "null", "{}", string(valid) + "{}", strings.Repeat("x", 4097), strings.Replace(string(valid), `"bucket":`, `"unexpected":`, 1), strings.Replace(string(valid), s.ConfigurationVersion, "", 1), strings.Replace(string(valid), s.ReleaseSHA256, "bad", 1)} {
		if _, err := parseSettings(raw); err == nil {
			t.Fatalf("accepted invalid bootstrap %q", raw)
		}
	}
	got, err := parseSettings(string(valid))
	if err != nil || got != s {
		t.Fatalf("valid bootstrap: %+v %v", got, err)
	}
}

func TestBootstrapUsesOnlyMatchingManagedPrincipal(t *testing.T) {
	for _, kind := range []string{"valid", "version", "token-path", "key-path", "provider", "tenant", "tenant-error", "region", "region-error", "bucket"} {
		t.Run(kind, func(t *testing.T) {
			s, _ := fixture(t)
			env := map[string]string{auth.ResourcePrincipalVersionEnvVar: "2.2", auth.ResourcePrincipalRPSTEnvVar: "/synthetic/token", auth.ResourcePrincipalPrivatePEMEnvVar: "/synthetic/key"}
			p := &testPrincipal{tenant: s.TenancyID, region: s.Region}
			var providerError error
			switch kind {
			case "version":
				env[auth.ResourcePrincipalVersionEnvVar] = "1.1"
			case "token-path":
				env[auth.ResourcePrincipalRPSTEnvVar] = "inline-token"
			case "key-path":
				env[auth.ResourcePrincipalPrivatePEMEnvVar] = "inline-key"
			case "provider":
				providerError = errors.New("private-data")
			case "tenant":
				p.tenant = "other"
			case "tenant-error":
				p.tenantError = errors.New("private-data")
			case "region":
				p.region = "other"
			case "region-error":
				p.regionError = errors.New("private-data")
			case "bucket":
				s.Bucket = "/invalid"
			}
			calls := 0
			store, err := connectPrincipal(s, func(k string) string { return env[k] }, func() (common.ConfigurationProvider, error) { calls++; return p, providerError })
			if (err == nil) != (kind == "valid") || (store != nil) != (kind == "valid") {
				t.Fatalf("unexpected connection %T %v", store, err)
			}
			if (kind == "version" || kind == "token-path" || kind == "key-path") && calls != 0 {
				t.Fatal("invalid identity invoked credential provider")
			}
		})
	}
}

func TestManagedBootstrapDoesNotFallbackToUserProfile(t *testing.T) {
	t.Setenv(auth.ResourcePrincipalVersionEnvVar, "2.2")
	t.Setenv(auth.ResourcePrincipalRPSTEnvVar, "/does-not-exist/router-token")
	t.Setenv(auth.ResourcePrincipalPrivatePEMEnvVar, "/does-not-exist/router-key")
	t.Setenv(auth.ResourcePrincipalRegionEnvVar, "us-ashburn-1")
	s, _ := fixture(t)
	if store, err := connect(s); err == nil || store != nil {
		t.Fatal("missing managed credentials accepted")
	}
}

type testPrincipal struct {
	common.ConfigurationProvider
	tenant, region           string
	tenantError, regionError error
}

func (p *testPrincipal) TenancyOCID() (string, error) { return p.tenant, p.tenantError }
func (p *testPrincipal) Region() (string, error)      { return p.region, p.regionError }
