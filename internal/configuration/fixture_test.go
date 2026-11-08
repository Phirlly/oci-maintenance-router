package configuration

import (
	"encoding/json"
	"errors"
	"os"
	"testing"
)

func catalog() Catalog {
	return Catalog{
		Regions:       map[string]string{"us-ashburn-1": "iad", "us-phoenix-1": "phx"},
		MySQLServices: []string{"Oracle HeatWave MySQL", "MySQL Database"},
	}
}

func fixture(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile("testdata/valid.json")
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func settings(t *testing.T) map[string]any {
	t.Helper()
	var value map[string]any
	if err := json.Unmarshal(fixture(t), &value); err != nil {
		t.Fatal(err)
	}
	return value
}

func encoded(t *testing.T, value map[string]any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func decoded(t *testing.T, value map[string]any) Config {
	t.Helper()
	got, err := Decode(encoded(t, value), catalog())
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func wantError(t *testing.T, data []byte, field, code string) {
	t.Helper()
	got, err := Decode(data, catalog())
	var detail *Error
	if !errors.As(err, &detail) {
		t.Fatalf("expected controlled error, got config=%+v err=%v", got, err)
	}
	if detail.Field != field || detail.Code != code {
		t.Fatalf("error = %v, want %s: %s", err, field, code)
	}
	if got.SchemaVersion != 0 {
		t.Fatal("failure returned usable partial configuration")
	}
}
