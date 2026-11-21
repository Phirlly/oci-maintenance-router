package configuration

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestSelectedCatalogOwnsClosedTypedServices(t *testing.T) {
	b := selectionFixture(t, "selected-catalog")
	c, err := DecodeCatalog(b)
	if err != nil || c.SchemaVersion != 1 || c.MySQLServices != nil || len(c.ResourceServices) != 2 {
		t.Fatal("typed catalog rejected or mixed with legacy services", err)
	}
	clear(b)
	c.ResourceServices["instance"][0] = "changed"
	c.Regions["us-ashburn-1"] = "changed"
	again := selectionCatalog(t)
	if again.ResourceServices["instance"][0] != "Oracle Cloud Infrastructure Compute" || again.Regions["us-ashburn-1"] != "iad" {
		t.Fatal("catalog instances share mutable values")
	}
}

func TestSelectedCatalogRejectsInvalidShapesWithoutPartialResults(t *testing.T) {
	valid := string(selectionFixture(t, "selected-catalog"))
	for _, change := range [][2]string{
		{`"schemaVersion":1`, `"schemaVersion":null`},
		{`"schemaVersion":1`, `"schemaVersion":"1"`},
		{`"schemaVersion":1`, `"schemaVersion":1.0`},
		{`"schemaVersion":1`, `"schemaVersion":1e0`},
		{`"schemaVersion":1`, `"schemaVersion":2`},
		{`"schemaVersion":1`, `"schemaVersion":[]`},
		{`"schemaVersion":1`, `"schemaVersion":{}`},
		{`"schemaVersion":1,`, ``},
		{`"schemaVersion":1`, `"schemaVersion":1,"mysqlServices":[]`},
		{`"resourceServices":`, `"resourceServices":{},"resourceServices":`},
		{`"resourceServices"`, `"ResourceServices"`},
		{`"instance"`, `"private-type"`},
		{`"instance":`, `"private-type":[],"instance":`},
		{`["Oracle Cloud Infrastructure Compute"]`, `[]`},
		{`["Oracle Cloud Infrastructure Compute"]`, `null`},
		{`["Oracle Cloud Infrastructure Compute"]`, `[12]`},
		{`["Oracle Cloud Infrastructure Compute"]`, `[""]`},
		{`["Oracle Cloud Infrastructure Compute"]`, `["duplicate","duplicate"]`},
		{`["Oracle Cloud Infrastructure Compute"]`, `["\ud800"]`},
		{`"iad"`, `"phx"`},
		{`"iad"`, `12`},
	} {
		input := strings.Replace(valid, change[0], change[1], 1)
		got, err := DecodeCatalog([]byte(input))
		var controlled *Error
		if !errors.As(err, &controlled) || !reflect.DeepEqual(got, Catalog{}) {
			t.Fatalf("invalid catalog returned data: %v", err)
		}
		if strings.Contains(err.Error(), "private-type") {
			t.Fatal("unknown catalog field escaped controlled errors")
		}
	}
}

func TestSelectedCatalogValidatesDirectTrustedValues(t *testing.T) {
	for _, edit := range []func(*Catalog){
		func(c *Catalog) { c.SchemaVersion = 2 },
		func(c *Catalog) { c.SchemaVersion = 0 },
		func(c *Catalog) { c.MySQLServices = []string{} },
		func(c *Catalog) { delete(c.ResourceServices, "instance") },
		func(c *Catalog) { c.ResourceServices["private-type"] = []string{"private-service"} },
		func(c *Catalog) { c.ResourceServices["instance"] = nil },
		func(c *Catalog) { c.ResourceServices["instance"] = []string{"a", "a"} },
		func(c *Catalog) { c.ResourceServices["instance"] = []string{string([]byte{255})} },
	} {
		cat := selectionCatalog(t)
		edit(&cat)
		c, err := Decode(selectionFixture(t, "selected"), cat)
		if err == nil || !reflect.DeepEqual(c, Config{}) || strings.Contains(err.Error(), "private") {
			t.Fatal("invalid catalog accepted or exposed input", err)
		}
	}
}
