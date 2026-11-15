package infra_test

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestClosureIntentRefusesMalformedOrUnacknowledgedAuthority(t *testing.T) {
	for _, kind := range []string{"mode", "missing-number", "zero", "fraction", "large", "code", "kind", "version", "null", "numeric-hash", "extra", "duplicate", "reordered", "whitespace", "padding", "notice"} {
		t.Run(kind, func(t *testing.T) {
			s := closureSettings()
			v := closureIntent()
			switch kind {
			case "mode":
				s["mode"] = "PREVIEW"
			case "missing-number":
				delete(s, "closure_request_number")
			case "zero":
				s["closure_request_number"] = 0
			case "fraction":
				s["closure_request_number"] = 1.5
			case "large":
				s["closure_request_number"] = 2147483648
			case "kind":
				v["kind"] = "RETRY_BATCH"
			case "version":
				v["version"] = "1"
			case "null":
				v["targetId"] = nil
			case "numeric-hash":
				v["targetId"] = 123
			case "extra":
				v["componentFingerprint"] = strings.Repeat("a", 64)
			case "notice":
				v["noticeId"] = "wrong"
			}
			code := closureCode(v)
			body, _ := base64.StdEncoding.DecodeString(code)
			switch kind {
			case "code":
				code = "invalid"
			case "duplicate":
				body = []byte(strings.Replace(string(body), `"version":1`, `"version":1,"version":1`, 1))
				code = base64.StdEncoding.EncodeToString(body)
			case "reordered":
				body = []byte(strings.Replace(string(body), `{"kind":`, `{"version":1,"kind":`, 1))
				body = []byte(strings.TrimSuffix(string(body), `,"version":1}`) + "}")
				code = base64.StdEncoding.EncodeToString(body)
			case "whitespace":
				code = base64.StdEncoding.EncodeToString(append(body, ' '))
			case "padding":
				code += "\n"
			}
			s["closure_review_code"] = code
			_, message, err := render(t, s)
			if err == nil || !strings.Contains(message, "Closure") {
				t.Fatalf("invalid %s admitted: %s (%v)", kind, message, err)
			}
		})
	}
}

func TestUncheckedClosureRetainsNoIntent(t *testing.T) {
	for _, mode := range []string{"PREVIEW", "PAUSED"} {
		s := settingsFixture()
		s["mode"] = mode
		before, message, err := render(t, s)
		if err != nil {
			t.Fatal(message, err)
		}
		s["closure_confirm"] = false
		s["closure_review_code"] = "stale malformed code"
		s["closure_request_number"] = 0
		after, message, err := render(t, s)
		if err != nil || before["configuration"] != after["configuration"] {
			t.Fatal("inactive closure changed configuration", message, err)
		}
	}
}
