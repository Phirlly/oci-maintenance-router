package configuration

import (
	"strings"
	"testing"
)

const catalogWire = `{"regions":{"us-ashburn-1":"iad"},"mysqlServices":["Oracle HeatWave MySQL"]}`

func TestCatalogDecodeOwnsIndependentWireValues(t *testing.T) {
	input := []byte(catalogWire)
	got, err := DecodeCatalog(input)
	if err != nil || got.Regions["us-ashburn-1"] != "iad" || len(got.MySQLServices) != 1 || got.MySQLServices[0] != "Oracle HeatWave MySQL" {
		t.Fatal("valid independent catalog rejected", err)
	}
	clear(input)
	if got.Regions["us-ashburn-1"] != "iad" {
		t.Fatal("caller buffer retained")
	}
	got.Regions["us-ashburn-1"] = "other"
	got.MySQLServices[0] = "other"
	again, err := DecodeCatalog([]byte(catalogWire))
	if err != nil || again.Regions["us-ashburn-1"] != "iad" || again.MySQLServices[0] != "Oracle HeatWave MySQL" {
		t.Fatal("catalog instances share mutable state")
	}
}

func TestCatalogDecodeRejectsAmbiguousOrInvalidInput(t *testing.T) {
	for _, input := range []string{
		`null`, `[]`, `{}`, `{"regions":{},"mysqlServices":[]}`,
		strings.Replace(catalogWire, `"regions"`, `"Regions"`, 1),
		strings.Replace(catalogWire, `"iad"`, `42`, 1),
		strings.Replace(catalogWire, `"iad"`, `"IAD"`, 1),
		strings.Replace(catalogWire, `"us-ashburn-1"`, `"private invalid region"`, 1),
		strings.Replace(catalogWire, `"iad"`, `"iad","us-phoenix-1":"iad"`, 1),
		strings.Replace(catalogWire, `"regions":`, `"regions":{},"regions":`, 1),
		strings.Replace(catalogWire, `"mysqlServices":`, `"unexpected":true,"mysqlServices":`, 1),
		strings.Replace(catalogWire, `["Oracle HeatWave MySQL"]`, `null`, 1),
		strings.Replace(catalogWire, `["Oracle HeatWave MySQL"]`, `[42]`, 1),
		strings.Replace(catalogWire, `["Oracle HeatWave MySQL"]`, `[""]`, 1),
		strings.Replace(catalogWire, `["Oracle HeatWave MySQL"]`, `["duplicate","duplicate"]`, 1),
		strings.Replace(catalogWire, `"iad"`, `"\ud800"`, 1),
		catalogWire + `{}`, strings.Repeat(" ", 1<<20) + catalogWire,
	} {
		got, err := DecodeCatalog([]byte(input))
		if err == nil || got.Regions != nil || got.MySQLServices != nil {
			t.Fatal("invalid catalog returned partial values")
		}
		if strings.Contains(err.Error(), "private") {
			t.Fatal("input text escaped diagnostics")
		}
	}
}
