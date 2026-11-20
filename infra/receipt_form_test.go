package infra_test

import (
	"strings"
	"testing"
)

func TestReceiptFormKeepsInactiveCodesFromBlockingRouting(t *testing.T) {
	f := readForm(t)
	for _, name := range []string{"test_receipt_code", "operator_receipt_code"} {
		v := f.Variables[name]
		if v.Type != "string" || v.Required || v.Default != "" {
			t.Fatalf("%s needs optional string input with explicit action validation", name)
		}
		if v.MaxLength != 0 || v.Pattern != "" {
			t.Fatal("inactive code must not block pausing; Terraform validates TEST receipt syntax")
		}
		for _, hint := range []string{"code", "same test number", "PagerDuty"} {
			if !strings.Contains(v.Description, hint) {
				t.Fatalf("%s hint must explain %s", name, hint)
			}
		}
	}
}
