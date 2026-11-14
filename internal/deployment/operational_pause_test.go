package deployment

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

func configureOperational(t *testing.T, h *Handler, s Settings, objects *operationalStore, edit func(map[string]any)) Settings {
	t.Helper()
	var value map[string]any
	if err := json.Unmarshal(objects.pinned.config, &value); err != nil {
		t.Fatal(err)
	}
	edit(value)
	objects.pinned.config, _ = json.Marshal(value)
	s.ConfigurationSHA256 = digestFixture(objects.pinned.config)
	raw, _ := json.Marshal(s)
	h.getenv = func(string) string { return string(raw) }
	return s
}

func TestPausedWorkerVerifiesCurrentConfigurationAndReleasesOwnership(t *testing.T) {
	for _, operation := range []string{"deployment-check", "reconcile"} {
		t.Run(operation, func(t *testing.T) {
			h, s, objects, _ := operationalFixture(t)
			s = configureOperational(t, h, s, objects, func(v map[string]any) { v["mode"] = "PAUSED" })
			body, expected := invocation(s), "operational-verified"
			if operation == "reconcile" {
				body, expected = `{"operation":"reconcile"}`, "idle"
			}
			for i := 0; i < 2; i++ {
				out := httptest.NewRecorder()
				h.Serve(operationalContext(t), strings.NewReader(body), out)
				var got OperationalResult
				if json.Unmarshal(out.Body.Bytes(), &got) != nil || got.Status != expected || got.ResponseVersion != 2 || got.Mode != "PAUSED" || got.ScanCompleted || got.Diagnostic != nil || got.Operator != nil || got.Processing != nil || got.CleanupCode != "" {
					t.Fatalf("paused invocation did not verify and release: %s", out.Body)
				}
			}
			if len(objects.objects) != 1 || objects.version != 4 {
				t.Fatalf("paused worker changed business state: %d objects/%d writes", len(objects.objects), objects.version)
			}
		})
	}
}
