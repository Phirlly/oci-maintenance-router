package deployment

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestCheckRequiresIdentityAndPreview(t *testing.T) {
	for _, kind := range []string{"tenant", "region", "live", "location"} {
		s, store := fixture(t)
		switch kind {
		case "tenant":
			s.TenancyID = "ocid1.tenancy.oc1..different"
		case "region":
			s.Region = "us-phoenix-1"
		case "live":
			var v map[string]any
			_ = json.Unmarshal(store.config, &v)
			v["mode"] = "LIVE"
			v["activeBaselineId"] = strings.Repeat("a", 64)
			store.config, _ = json.Marshal(v)
			s.ConfigurationSHA256 = digestFixture(store.config)
		case "location":
			s.Namespace = ""
		}
		got := check(testContext(t), s, store, testAttemptID)
		if got.Status != "not-ready" || store.creates != 0 {
			t.Fatalf("unsafe %s: %+v", kind, got)
		}
	}
}

func TestPinnedReadRejectsOversizedOrUnavailableContent(t *testing.T) {
	s, store := fixture(t)
	store.config = []byte(strings.Repeat("x", (1<<20)+1))
	s.ConfigurationSHA256 = digestFixture(store.config)
	if result := check(testContext(t), s, store, testAttemptID); result.Code != "configuration_read" {
		t.Fatal(result)
	}
	store.readError = errors.New("private-data")
	if _, err := readPinned(testContext(t), store, "configuration/router.json", s.ConfigurationVersion, s.ConfigurationSHA256); err == nil {
		t.Fatal("accepted unavailable content")
	}
}
