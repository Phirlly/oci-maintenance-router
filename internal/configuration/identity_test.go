package configuration

import (
	"bytes"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func generation(t *testing.T, c Config) string {
	t.Helper()
	id, err := c.RoutingGeneration()
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestRoutingGoldenEncoding(t *testing.T) {
	c := decoded(t, settings(t))
	got, err := c.canonicalRouting()
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile("testdata/routing-canonical.json")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, bytes.TrimSuffix(want, []byte("\n"))) {
		t.Fatalf("canonical bytes:\n%s\nwant:\n%s", got, want)
	}
	if got := generation(t, c); got != "2cc11f0431bdd77c4232e1274c22d93f4e57f46d7fa35e352201a3296022aee7" {
		t.Fatalf("generation = %s", got)
	}
	c.Routes[0].OwnerValue = "<>&\u2028\u2029🐬"
	got, err = c.canonicalRouting()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(got, []byte(`"ownerValue":"\u003c\u003e\u0026\u2028\u2029🐬"`)) {
		t.Fatalf("canonical escape contract changed: %s", got)
	}
	c.Ownership = Ownership{Kind: "freeform", Key: "Owner"}
	got, err = c.canonicalRouting()
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(got, []byte("namespace")) {
		t.Fatalf("freeform included namespace: %s", got)
	}
}

func TestRoutingOrderIndependenceWithoutMutation(t *testing.T) {
	all := settings(t)
	all["compartmentIds"] = []any{"ocid1.compartment.oc1..z", "ocid1.compartment.oc1..a"}
	all["mysqlServices"] = []any{"Oracle HeatWave MySQL", "MySQL Database"}
	all["destinations"] = []any{
		map[string]any{"id": "z", "secretId": "ocid1.vaultsecret.oc1.iad.z"},
		map[string]any{"id": "payments", "secretId": "ocid1.vaultsecret.oc1.iad.example"},
	}
	c := decoded(t, all)
	before := decoded(t, all)
	want := generation(t, c)
	if !reflect.DeepEqual(c, before) {
		t.Fatal("identity sorted caller-owned data")
	}
	slices.Reverse(c.CompartmentIDs)
	slices.Reverse(c.MySQLServices)
	slices.Reverse(c.Destinations)
	slices.Reverse(c.Routes)
	if got := generation(t, c); got != want {
		t.Fatalf("order changed generation: %s != %s", got, want)
	}
}

func TestEveryRoutingInputChangesGeneration(t *testing.T) {
	for name, change := range map[string]func(*Config){
		"schema":            func(c *Config) { c.SchemaVersion = 2 },
		"tenancy":           func(c *Config) { c.TenancyID = "ocid1.tenancy.oc1..other" },
		"region":            func(c *Config) { c.DeploymentRegion = "us-phoenix-1" },
		"region scope":      func(c *Config) { delete(c.RegionMap, "us-phoenix-1") },
		"compartments":      func(c *Config) { c.CompartmentIDs[0] = "ocid1.compartment.oc1..other" },
		"services":          func(c *Config) { c.MySQLServices[0] = "MySQL Database" },
		"kind":              func(c *Config) { c.Ownership = Ownership{Kind: "freeform", Key: "Owner"} },
		"namespace":         func(c *Config) { c.Ownership.Namespace = "Other" },
		"key":               func(c *Config) { c.Ownership.Key = "owner" },
		"destination id":    func(c *Config) { c.Destinations[0].ID = "other" },
		"secret reference":  func(c *Config) { c.Destinations[0].SecretID = "ocid1.vaultsecret.oc1.iad.other" },
		"owner":             func(c *Config) { c.Routes[0].OwnerValue = "Payments" },
		"route destination": func(c *Config) { c.Routes[0].DestinationID = "other" },
		"pd region":         func(c *Config) { c.PagerDutyRegion = "EU" },
	} {
		t.Run(name, func(t *testing.T) {
			c := decoded(t, settings(t))
			before := generation(t, c)
			change(&c)
			if generation(t, c) == before {
				t.Fatal("routing change did not change identity")
			}
		})
	}
}

func TestOperationalControlsDoNotChangeRoutingGeneration(t *testing.T) {
	c := decoded(t, settings(t))
	before := generation(t, c)
	c.EnvironmentName = "renamed"
	c.Mode = "LIVE"
	c.ActiveBaselineID = "baseline-2"
	c.Replay = []Replay{{"ocid1.announcement.oc1..example", strings.Repeat("a", 64)}}
	c.IntakeTopicID = "ocid1.onstopic.oc1.iad.otherintake"
	c.OperatorTopicID = "ocid1.onstopic.oc1.iad.otheroperator"
	if generation(t, c) != before {
		t.Fatal("operational change affected routing generation")
	}
}
