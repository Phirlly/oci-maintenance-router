package deployment

import "testing"

func TestCatalogRejectsMalformedOrTrailingJSONBeforeConfigurationRead(t *testing.T) {
	for _, suffix := range []string{"{}", " trailing"} {
		s, store := fixture(t)
		store.catalog = append(store.catalog, []byte(suffix)...)
		s.CatalogSHA256 = digestFixture(store.catalog)
		result := check(testContext(t), s, store, testAttemptID)
		if result.Code != "catalog_invalid" || store.reads != 1 || store.creates != 0 {
			t.Fatalf("invalid catalog accepted: %+v", result)
		}
	}
}
