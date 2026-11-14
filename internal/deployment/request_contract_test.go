package deployment

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestReconcileUsesTrustedConfigurationAndRejectsLegacyRuntime(t *testing.T) {
	h, _, store := handlerFixture(t)
	out := httptest.NewRecorder()
	h.Serve(testContext(t), strings.NewReader(`{"operation":"reconcile"}`), out)
	var got Result
	if json.Unmarshal(out.Body.Bytes(), &got) != nil || got.Code != "configuration_unsupported" || store.reads != 2 || store.creates != 0 {
		t.Fatalf("reconcile did not verify trusted legacy configuration: %s; reads=%d writes=%d", out.Body, store.reads, store.creates)
	}
}

func TestFunctionCommandsRejectDuplicateFieldsBeforeBootstrap(t *testing.T) {
	for _, field := range []string{"operation", "configurationSha256", "releaseSha256", "checkAttemptId"} {
		t.Run(field, func(t *testing.T) {
			h, s, store := handlerFixture(t)
			var request map[string]string
			if err := json.Unmarshal([]byte(invocation(s)), &request); err != nil {
				t.Fatal(err)
			}
			value, _ := json.Marshal(request[field])
			body := strings.TrimSuffix(invocation(s), "}") + `,"` + field + `":` + string(value) + "}"
			out := httptest.NewRecorder()
			h.Serve(testContext(t), strings.NewReader(body), out)
			var got Result
			if json.Unmarshal(out.Body.Bytes(), &got) != nil || got.Code != "request_invalid" || store.reads != 0 || store.creates != 0 {
				t.Fatalf("duplicate command reached trusted I/O: %s", out.Body)
			}
		})
	}
}

func TestReconcileRejectsInvocationAuthorityFields(t *testing.T) {
	for _, field := range []string{"mode", "configurationSha256", "releaseSha256", "checkAttemptId", "recovery", "diagnostic", "destination", "payload"} {
		t.Run(field, func(t *testing.T) {
			h, _, store := handlerFixture(t)
			body := `{"operation":"reconcile","` + field + `":"untrusted"}`
			out := httptest.NewRecorder()
			h.Serve(testContext(t), strings.NewReader(body), out)
			if !strings.Contains(out.Body.String(), `"code":"request_invalid"`) || store.reads != 0 || store.creates != 0 {
				t.Fatalf("invocation authority accepted: %s", out.Body)
			}
		})
	}
}

func TestCommandsRejectMalformedEncodingBeforeConnection(t *testing.T) {
	for _, body := range []string{
		`{"operation":"reconcile","oper\u0061tion":"reconcile"}`,
		`{"operation":null}`, `{"operation":{}}`, `{"operation":1}`,
		`{"operation":"reconcile",}`, `{"operation":"reconcile"`,
		`{"operation":"reconcile"} null`, `[]`,
		"{\"operation\":\"reconcile\xff\"}",
	} {
		h, _, _ := handlerFixture(t)
		h.connect = func(Settings) (Store, error) { t.Fatal("malformed command reached authentication"); return nil, nil }
		out := httptest.NewRecorder()
		h.Serve(testContext(t), strings.NewReader(body), out)
		if !strings.Contains(out.Body.String(), `"code":"request_invalid"`) {
			t.Fatalf("malformed command accepted: %q: %s", body, out.Body)
		}
	}
}
