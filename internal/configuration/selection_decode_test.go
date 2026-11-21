package configuration

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestSelectedConfigurationAdmitsExplicitResourceChoices(t *testing.T) {
	for _, kinds := range [][]string{{"mysqldbsystem"}, {"instance"}, {"mysqldbsystem", "instance"}} {
		v := selectionSettings(t)
		v["resourceTypes"] = kinds
		c := selectedConfig(t, v)
		if c.SchemaVersion != 9 || c.Mode != "PREVIEW" || c.Setup == nil || c.Setup.Action != "NONE" {
			t.Fatal("resource selection lost operational controls")
		}
		if !reflect.DeepEqual(c.ResourceTypes, kinds) || len(c.ResourceServices) != len(kinds) || c.MySQLServices != nil {
			t.Fatal("selected types or their binding changed")
		}
		for _, kind := range kinds {
			if !reflect.DeepEqual(c.ResourceServices[kind], selectionCatalog(t).ResourceServices[kind]) {
				t.Fatal("selection lost trusted service association")
			}
		}
	}
}

func TestSelectedConfigurationOwnsDerivedCatalogValues(t *testing.T) {
	cat := selectionCatalog(t)
	b := selectionFixture(t, "selected")
	c, err := Decode(b, cat)
	if err != nil {
		t.Fatal(err)
	}
	before := generation(t, c)
	clear(b)
	cat.ResourceServices["instance"][0] = "changed"
	delete(cat.ResourceServices, "mysqldbsystem")
	if generation(t, c) != before {
		t.Fatal("catalog mutated decoded configuration")
	}
	c.ResourceServices["instance"][0] = "local"
	if cat.ResourceServices["instance"][0] != "changed" {
		t.Fatal("configuration mutated catalog")
	}
}

func TestSelectedConfigurationRejectsAmbiguousSelection(t *testing.T) {
	for _, edit := range []func(map[string]any){
		func(v map[string]any) { delete(v, "resourceTypes") },
		func(v map[string]any) { v["resourceTypes"] = nil },
		func(v map[string]any) { v["resourceTypes"] = "instance" },
		func(v map[string]any) { v["resourceTypes"] = []any{12} },
		func(v map[string]any) { v["resourceTypes"] = []string{} },
		func(v map[string]any) { v["resourceTypes"] = []string{"instance", "instance"} },
		func(v map[string]any) { v["resourceTypes"] = []string{"private-kind"} },
		func(v map[string]any) { v["resourceTypes"] = []string{"INSTANCE"} },
		func(v map[string]any) { v["resourceTypes"] = []string{" instance"} },
		func(v map[string]any) { v["mysqlServices"] = []string{"Oracle HeatWave MySQL"} },
		func(v map[string]any) { v["resourceServices"] = map[string]any{} },
	} {
		v := selectionSettings(t)
		edit(v)
		c, err := Decode(encoded(t, v), selectionCatalog(t))
		var controlled *Error
		if !errors.As(err, &controlled) || !reflect.DeepEqual(c, Config{}) || strings.Contains(err.Error(), "private") {
			t.Fatal("invalid selection was usable or exposed input", err)
		}
	}
}

func TestSelectedConfigurationRequiresExactCatalogPairing(t *testing.T) {
	for _, tc := range []struct {
		data []byte
		cat  Catalog
	}{
		{selectionFixture(t, "selected"), catalog()},
		{fixture(t), selectionCatalog(t)},
		{encoded(t, setupSettings(t, "PREVIEW", "NONE")), selectionCatalog(t)},
	} {
		c, err := Decode(tc.data, tc.cat)
		var detail *Error
		if !errors.As(err, &detail) || detail.Field != "catalog" || detail.Code != "incompatible" || !reflect.DeepEqual(c, Config{}) {
			t.Fatal("incompatible catalog/config pairing accepted", err)
		}
	}
	legacy := catalog()
	legacy.ResourceServices = map[string][]string{}
	if _, err := Decode(fixture(t), legacy); err == nil {
		t.Fatal("mixed legacy catalog accepted")
	}
}
