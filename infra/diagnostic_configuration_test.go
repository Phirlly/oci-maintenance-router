package infra_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"oci-maintenance-router/internal/configuration"
)

func diagnosticSettings() map[string]any {
	s := settingsFixture()
	s["mode"] = "TEST"
	s["test_secret_ocid"] = "ocid1.vaultsecret.oc1.iad.nonproduction"
	s["test_nonproduction"] = true
	s["test_run"] = 5
	return s
}

func renderedConfig(t *testing.T, values map[string]string) configuration.Config {
	t.Helper()
	var catalog configuration.Catalog
	if err := json.Unmarshal([]byte(values["catalog"]), &catalog); err != nil {
		t.Fatal(err)
	}
	c, err := configuration.Decode([]byte(values["configuration"]), catalog)
	if err != nil {
		t.Fatalf("runtime rejected generated configuration: %v", err)
	}
	return c
}

func TestRenderedOperationalModesKeepTestAuthoritySeparate(t *testing.T) {
	for _, mode := range []string{"PREVIEW", "PAUSED", "TEST"} {
		t.Run(mode, func(t *testing.T) {
			s := diagnosticSettings()
			s["mode"] = mode
			values, message, err := render(t, s)
			if err != nil {
				t.Fatalf("render: %s (%v)", message, err)
			}
			c := renderedConfig(t, values)
			if c.Mode != mode || c.SchemaVersion != 7 || c.OperatorTest != nil || c.Recovery != nil || c.ActiveBaselineID != "" {
				t.Fatalf("unexpected mode/authority: %s", c.Mode)
			}
			if mode != "TEST" {
				if c.Diagnostic != nil {
					t.Fatal("inactive test fields granted diagnostic authority")
				}
				return
			}
			// Independently spell the identity tuple; routing and secrets are not identity inputs.
			sum := sha256.Sum256([]byte(`["diagnostic-request-v1","b1-` + strings.Repeat("a", 64) + `",5]`))
			want := &configuration.DiagnosticRequest{RequestID: "d1-" + hex.EncodeToString(sum[:]), SecretID: s["test_secret_ocid"].(string), NonProduction: true}
			if !reflect.DeepEqual(c.Diagnostic, want) || c.Destinations[0].SecretID == c.Diagnostic.SecretID {
				t.Fatalf("test intent/secret isolation differs: %#v", c.Diagnostic)
			}
		})
	}
}

func TestOmittedOrNullOperationalControlsPreservePreview(t *testing.T) {
	for _, explicitNull := range []bool{false, true} {
		s := settingsFixture()
		if explicitNull {
			for _, key := range []string{"mode", "test_secret_ocid", "test_nonproduction", "test_run"} {
				s[key] = nil
			}
		}
		values, message, err := render(t, s)
		if err != nil {
			t.Fatalf("legacy/default render: %s (%v)", message, err)
		}
		c := renderedConfig(t, values)
		if c.Mode != "PREVIEW" || c.Diagnostic != nil {
			t.Fatal("omitted/null controls authorized testing")
		}
	}
}
