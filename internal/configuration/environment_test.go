package configuration

import (
	"strings"
	"testing"
)

func TestEnvironmentIdentityGoldenAndIsolation(t *testing.T) {
	base := LedgerLocation{"example_namespace", "Router.Ledger"}
	c := decoded(t, settings(t))
	want := "f8cffa22beb4df0f1675fa9016195e88248905435a8c65205e2916d8c6e2d8c0"
	id, err := c.EnvironmentID(base)
	if err != nil || id != want {
		t.Fatalf("identity=%s err=%v", id, err)
	}
	for _, location := range []LedgerLocation{
		{"other_namespace", base.Bucket}, {base.Namespace, "router.Ledger"},
		{"example", "namespace_Router.Ledger"},
	} {
		id, err := c.EnvironmentID(location)
		if err != nil || id == want {
			t.Fatalf("environment collision: %s %v", id, err)
		}
	}
	c.EnvironmentName = "renamed"
	c.Mode = "PAUSED"
	c.Routes[0].OwnerValue = "other"
	id, err = c.EnvironmentID(base)
	if err != nil || id != want {
		t.Fatalf("routing/control changed environment: %s %v", id, err)
	}
	for _, change := range []func(*Config){
		func(c *Config) { c.TenancyID = "ocid1.tenancy.oc1..other" },
		func(c *Config) { c.DeploymentRegion = "us-phoenix-1" },
	} {
		c = decoded(t, settings(t))
		change(&c)
		id, err = c.EnvironmentID(base)
		if err != nil || id == want {
			t.Fatalf("environment not isolated: %s %v", id, err)
		}
	}
}

func TestLedgerLocationValidation(t *testing.T) {
	c := decoded(t, settings(t))
	for _, location := range []LedgerLocation{
		{"", "ledger"}, {strings.Repeat("a", 1025), "ledger"}, {string([]byte{0xff}), "ledger"},
		{"namespace", ""}, {"namespace", strings.Repeat("a", 257)}, {"namespace", "bad/bucket"},
		{"namespace", "bad bucket"}, {"namespace", "🐬"},
	} {
		id, err := c.EnvironmentID(location)
		if err == nil || id != "" {
			t.Fatalf("accepted invalid location: id=%s err=%v", id, err)
		}
	}
	if _, err := c.EnvironmentID(LedgerLocation{strings.Repeat("a", 1024), strings.Repeat("a", 256)}); err != nil {
		t.Fatal(err)
	}
}
