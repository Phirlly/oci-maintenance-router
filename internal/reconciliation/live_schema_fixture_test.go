package reconciliation

import "testing"

// These are syntactically valid receipt inputs only. Each verifier test replaces
// its own code with the actual saved record's code; this is not combined readiness.
func liveSchemaConfig(t *testing.T, mode string, changes ...func(map[string]any)) []byte {
	t.Helper()
	return diagnosticConfig(t, append([]func(map[string]any){operatorTestConfig, func(v map[string]any) {
		v["schemaVersion"], v["mode"] = 6, mode
		v["diagnostic"].(map[string]any)["receiptCode"] = "OMR-0123456789abcdef0123456789abcdef"
		v["operatorTest"].(map[string]any)["receiptCode"] = "OMR-OP-0123456789abcdef0123456789abcdef"
	}}, changes...)...)
}
