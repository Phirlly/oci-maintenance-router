package infra_test

import "testing"

func TestSetupRejectsMissingOrConflictingIntentBeforeRendering(t *testing.T) {
	for _, tc := range []struct {
		name, mode, action, key string
		paired                  bool
		value                   any
	}{
		{"no attempt", "TEST", "SEND_TEST", "setup_attempt", true, nil},
		{"fraction", "TEST", "SEND_TEST", "setup_attempt", true, 1.5},
		{"overflow", "TEST", "SEND_TEST", "setup_attempt", true, 2147483648},
		{"no consent", "TEST", "SEND_TEST", "test_nonproduction", true, false},
		{"no secret", "TEST", "SEND_TEST", "test_secret_ocid", true, ""},
		{"missing PD code", "TEST", "CONFIRM", "test_receipt_code", true, ""},
		{"missing operator code", "TEST", "CONFIRM", "operator_receipt_code", true, ""},
		{"diagnostic missing code", "TEST", "CONFIRM", "test_receipt_code", false, ""},
		{"diagnostic orphan code", "TEST", "CONFIRM", "operator_receipt_code", false, "OMR-OP-invalid"},
		{"diagnostic missing secret", "TEST", "CONFIRM", "test_secret_ocid", false, ""},
		{"diagnostic missing consent", "TEST", "CONFIRM", "test_nonproduction", false, false},
		{"setup during live", "LIVE", "SEND_TEST", "setup_attempt", true, 1},
		{"no test action", "TEST", "NONE", "setup_attempt", true, 1},
		{"unknown action", "TEST", "unexpected", "setup_attempt", true, 1},
		{"closure mixed with setup", "TEST", "SEND_TEST", "closure_confirm", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := setupSettings(tc.mode, tc.action, tc.paired)
			s[tc.key] = tc.value
			if _, message, err := renderSetup(t, s); err == nil {
				t.Fatalf("invalid setup rendered: %s", message)
			}
		})
	}
}
