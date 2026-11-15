package infra_test

import "testing"

func TestClosureControlsStayVisibleAndDefaultToNoAction(t *testing.T) {
	f := readForm(t)
	for _, name := range []string{"closure_review_code", "closure_request_number", "closure_confirm"} {
		v, ok := f.Variables[name]
		if !ok || v.Visible != true || v.Required {
			t.Fatalf("closure field missing or hard to clear: %s", name)
		}
	}
	if f.Variables["closure_confirm"].Default != false || f.Variables["closure_review_code"].Default != "" || f.Variables["closure_review_code"].MaxLength != 6000 || f.Variables["closure_request_number"].Default != nil {
		t.Fatal("closure gained implicit authority")
	}
}
