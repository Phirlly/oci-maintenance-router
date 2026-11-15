package infra_test

import (
	"strings"
	"testing"
)

func TestReceiptFormLeavesInactiveAndOrphanCodesEditable(t *testing.T) {
	f := readForm(t)
	for _, name := range []string{"test_receipt_code", "operator_receipt_code"} {
		v := f.Variables[name]
		if v.Type != "string" || v.Required || v.Default != "" || v.Visible != true {
			t.Fatalf("%s must remain optional and visible for retention or clearing", name)
		}
		if v.MaxLength != 0 || v.Pattern != "" {
			t.Fatal("inactive code must not block pausing; Terraform validates TEST receipt syntax")
		}
		for _, hint := range []string{"code", "unchanged", "TEST"} {
			if !strings.Contains(v.Description, hint) {
				t.Fatalf("%s hint must explain %s", name, hint)
			}
		}
	}
}
