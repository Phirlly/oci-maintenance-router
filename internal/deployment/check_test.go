package deployment

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"oci-maintenance-router/internal/ocistorage"
)

type memoryStore struct {
	config, catalog        []byte
	markers                map[string][]byte
	readOverride           []byte
	saveBeforeError        bool
	reads, creates         int
	readError, createError error
}

func (s *memoryStore) GetVersion(_ context.Context, name, version string) (ocistorage.Object, error) {
	s.reads++
	if s.readError != nil {
		return ocistorage.Object{}, s.readError
	}
	if name == "configuration/router.json" && version == "config-v1" {
		return ocistorage.Object{Body: s.config}, nil
	}
	if name == "configuration/catalog.json" && version == "catalog-v1" {
		return ocistorage.Object{Body: s.catalog}, nil
	}
	return ocistorage.Object{}, errors.New("unexpected object/version")
}
func (s *memoryStore) Create(_ context.Context, name string, body []byte) (string, error) {
	s.creates++
	if s.createError != nil && !s.saveBeforeError {
		return "", s.createError
	}
	if _, exists := s.markers[name]; exists {
		return "", &ocistorage.Error{Code: ocistorage.Conflict, Status: 412}
	}
	s.markers[name] = append([]byte(nil), body...)
	return "created", s.createError
}
func (s *memoryStore) Get(_ context.Context, name string) (ocistorage.Object, error) {
	if s.readOverride != nil {
		return ocistorage.Object{Body: s.readOverride}, s.readError
	}
	return ocistorage.Object{Body: append([]byte(nil), s.markers[name]...)}, s.readError
}
func digestFixture(b []byte) string { sum := sha256.Sum256(b); return hex.EncodeToString(sum[:]) }

func fixture(t *testing.T) (Settings, *memoryStore) {
	t.Helper()
	data, err := os.ReadFile("../configuration/testdata/valid.json")
	if err != nil {
		t.Fatal(err)
	}
	catalog := []byte(`{"regions":{"us-ashburn-1":"iad","us-phoenix-1":"phx"},"mysqlServices":["Oracle HeatWave MySQL"]}`)
	s := Settings{TenancyID: "ocid1.tenancy.oc1..example", Region: "us-ashburn-1", Namespace: "example", Bucket: "example-ledger", ConfigurationVersion: "config-v1", CatalogVersion: "catalog-v1", ConfigurationSHA256: digestFixture(data), CatalogSHA256: digestFixture(catalog), ReleaseSHA256: digestFixture([]byte("release"))}
	return s, &memoryStore{config: data, catalog: catalog, markers: make(map[string][]byte)}
}
func testContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	t.Cleanup(cancel)
	return ctx
}

func TestCheckIsNonsendingAndIdempotent(t *testing.T) {
	s, store := fixture(t)
	for i := 0; i < 2; i++ {
		result := check(testContext(t), s, store, testAttemptID)
		if result.Status != "deployment-verified" || result.SendingEnabled || result.ConfigurationSHA256 != s.ConfigurationSHA256 || result.ReleaseSHA256 != s.ReleaseSHA256 || len(result.RoutingGeneration) != 64 {
			t.Fatalf("result=%+v", result)
		}
	}
	if store.creates != 2 || store.reads != 4 || len(store.markers) != 1 {
		t.Fatalf("unexpected storage work: %+v", store)
	}
}

func TestCheckRejectsConfigurationBeforeWriting(t *testing.T) {
	for _, field := range []string{"mode", "tenancyId", "deploymentRegion", "activeBaselineId"} {
		t.Run(field, func(t *testing.T) {
			s, store := fixture(t)
			var v map[string]any
			_ = json.Unmarshal(store.config, &v)
			v[field] = "invalid"
			store.config, _ = json.Marshal(v)
			s.ConfigurationSHA256 = digestFixture(store.config)
			got := check(testContext(t), s, store, testAttemptID)
			if got.Status == "deployment-verified" || store.creates != 0 || got.SendingEnabled {
				t.Fatalf("accepted invalid config: %+v", got)
			}
		})
	}
}

func TestCheckRejectsUntrustedOrUnavailableObjects(t *testing.T) {
	for _, kind := range []string{"config-hash", "catalog-hash", "catalog-json", "read", "marker", "write"} {
		t.Run(kind, func(t *testing.T) {
			s, store := fixture(t)
			switch kind {
			case "config-hash":
				store.config = []byte("changed")
			case "catalog-hash":
				store.catalog = []byte("changed")
			case "catalog-json":
				store.catalog = []byte("{}")
				s.CatalogSHA256 = digestFixture(store.catalog)
			case "read":
				store.readError = errors.New("private-data")
			case "marker":
				store.readOverride = []byte("wrong")
			case "write":
				store.createError = errors.New("private-data")
			}
			got := check(testContext(t), s, store, testAttemptID)
			if got.Status == "deployment-verified" || got.SendingEnabled {
				t.Fatalf("unsafe result: %+v", got)
			}
			b, _ := json.Marshal(got)
			if strings.Contains(string(b), "private-data") {
				t.Fatal("raw error escaped")
			}
		})
	}
}
