package configuration

import (
	"bytes"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func TestSelectedRoutingGoldenAndIndependentEnvironment(t *testing.T) {
	c := selectedConfig(t, selectionSettings(t))
	got, err := c.canonicalRouting()
	want := bytes.TrimSpace(selectionFixture(t, "selected-routing-canonical"))
	if err != nil || !bytes.Equal(got, want) {
		t.Fatalf("selected projection changed: %s %v", got, err)
	}
	if generation(t, c) != "407cb2eefc0bc32866152e2c93926b5f27cff7807dfe1afa49a1d4113c8f3a4d" {
		t.Fatal("selected generation changed")
	}
	legacy := decoded(t, settings(t))
	location := LedgerLocation{Namespace: "example", Bucket: "router-state"}
	a, _ := c.EnvironmentID(location)
	b, _ := legacy.EnvironmentID(location)
	if a != b {
		t.Fatal("selection changed environment identity")
	}
	c.ResourceTypes = []string{"mysqldbsystem"}
	c.ResourceServices["mysqldbsystem"] = slices.Clone(legacy.MySQLServices)
	if generation(t, c) == generation(t, legacy) {
		t.Fatal("all-category meaning reused legacy identity")
	}
}

func TestSelectedRoutingIsOrderIndependentAndDoesNotMutate(t *testing.T) {
	v := selectionSettings(t)
	v["compartmentIds"] = []string{"ocid1.compartment.oc1..z", "ocid1.compartment.oc1..a"}
	c, before := selectedConfig(t, v), selectedConfig(t, v)
	want := generation(t, c)
	if !reflect.DeepEqual(c, before) {
		t.Fatal("generation mutated caller")
	}
	slices.Reverse(c.ResourceTypes)
	slices.Reverse(c.ResourceServices["mysqldbsystem"])
	slices.Reverse(c.CompartmentIDs)
	slices.Reverse(c.Routes)
	slices.Reverse(c.Destinations)
	if generation(t, c) != want {
		t.Fatal("ordering changed meaning")
	}
}

func TestSelectedTypesAndTypedServiceBindingChangeGeneration(t *testing.T) {
	for _, edit := range []func(*Config){
		func(c *Config) { c.ResourceTypes = []string{"instance"} },
		func(c *Config) { c.ResourceTypes = []string{"mysqldbsystem"} },
		func(c *Config) { c.ResourceServices["instance"] = []string{"approved alternate"} },
		func(c *Config) { c.ResourceServices["mysqldbsystem"][0] = "approved alternate" },
		func(c *Config) {
			c.ResourceServices["instance"], c.ResourceServices["mysqldbsystem"] = c.ResourceServices["mysqldbsystem"], c.ResourceServices["instance"]
		},
	} {
		c := selectedConfig(t, selectionSettings(t))
		before := generation(t, c)
		edit(&c)
		if generation(t, c) == before {
			t.Fatal("changed selection/binding lost from identity")
		}
	}
	v := selectionSettings(t)
	v["resourceTypes"] = []string{"instance"}
	cat := selectionCatalog(t)
	first, err := Decode(encoded(t, v), cat)
	if err != nil {
		t.Fatal(err)
	}
	cat.ResourceServices["mysqldbsystem"] = []string{"other approved alias"}
	second, err := Decode(encoded(t, v), cat)
	if err != nil {
		t.Fatal(err)
	}
	if generation(t, first) != generation(t, second) {
		t.Fatal("unselected catalog affected routing")
	}
}

func TestSelectedOperationalControlsDoNotChangeRouting(t *testing.T) {
	want := generation(t, selectedConfig(t, selectionSettings(t)))
	for _, tc := range []struct{ mode, action string }{{"PREVIEW", "NONE"}, {"PAUSED", "NONE"}, {"LIVE", "NONE"}, {"TEST", "SEND_TEST"}, {"TEST", "CONFIRM"}} {
		v := setupSettings(t, tc.mode, tc.action)
		v["schemaVersion"], v["resourceTypes"] = 9, []string{"instance", "mysqldbsystem"}
		delete(v, "mysqlServices")
		c := selectedConfig(t, v)
		c.EnvironmentName = "renamed"
		c.InstallationID = "other"
		c.Preparation.Initialize = !c.Preparation.Initialize
		c.IntakeTopicID = "ocid1.onstopic.oc1.iad.changed"
		c.OperatorTopicID = "ocid1.onstopic.oc1.iad.changedoperator"
		c.Recovery = &RecoveryRequest{RequestID: "review", TargetID: strings.Repeat("a", 64)}
		if generation(t, c) != want {
			t.Fatal("operational controls changed routing")
		}
	}
}
