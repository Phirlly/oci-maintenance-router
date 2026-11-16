package deployment

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"oci-maintenance-router/internal/ocistorage"
)

type failedPinnedStore struct {
	Store
	name  string
	err   error
	body  []byte
	reads int
}

func (s *failedPinnedStore) GetVersion(ctx context.Context, name, version string) (ocistorage.Object, error) {
	s.reads++
	if name == s.name {
		return ocistorage.Object{Body: s.body}, s.err
	}
	return s.Store.GetVersion(ctx, name, version)
}

// Exercise the real startup and logging path; assert its public result stays fixed.
func observePinnedFailure(t *testing.T, stage string, err error, body []byte) map[string]any {
	t.Helper()
	h, settings, objects, _ := operationalFixture(t)
	name, reads := "configuration/catalog.json", 1
	if stage == "configuration" {
		name, reads = "configuration/router.json", 2
	}
	store := &failedPinnedStore{Store: objects, name: name, err: err, body: body}
	h.connect = func(Settings) (Store, error) { return store, nil }
	h.compose = func(Settings, Store, pinnedInputs, string) (operations, error) {
		t.Fatal("failed pinned read reached downstream operations")
		return operations{}, nil
	}
	h.metrics = func(Settings, Store, string) (metricPublisher, error) {
		t.Fatal("failed pinned read reached metrics")
		return nil, nil
	}
	var logs bytes.Buffer
	h.logger = log.New(&logs, "", 0)
	out := httptest.NewRecorder()
	h.Serve(operationalContext(t), strings.NewReader(invocation(settings)), out)
	var result, line map[string]any
	want := map[string]any{"status": "not-ready", "code": stage + "_read", "sendingEnabled": false, "checkAttemptId": testAttemptID}
	if json.Unmarshal(out.Body.Bytes(), &result) != nil || !reflect.DeepEqual(result, want) || out.Code != 400 {
		t.Fatalf("changed public failure response: %s", out.Body)
	}
	if store.reads != reads || objects.version != 0 || objects.pinned.creates != 0 {
		t.Fatal("failure caused extra reads, retries or writes", store.reads, objects.version)
	}
	if json.Unmarshal(logs.Bytes(), &line) != nil || line["code"] != stage+"_read" || line["status"] != "not-ready" || line["scanCompleted"] != false || bytes.Count(logs.Bytes(), []byte("\n")) != 1 || logs.Len() > 512 {
		t.Fatal("missing single bounded failure log", logs.String())
	}
	for _, private := range []string{"private", "ocid1", "https://", name, testAttemptID, settings.ConfigurationSHA256, settings.CatalogSHA256} {
		if strings.Contains(logs.String(), private) {
			t.Fatal("log exposed private context", private)
		}
	}
	detail, ok := line["pinnedRead"].(map[string]any)
	if !ok {
		t.Fatal("missing pinned-read diagnostic", logs.String())
	}
	return detail
}

func TestPinnedReadLogDistinguishesStorageFailures(t *testing.T) {
	for _, stage := range []string{"catalog", "configuration"} {
		for _, tc := range []struct {
			code   ocistorage.Code
			status int
		}{
			{ocistorage.Authentication, 0}, {ocistorage.Transport, 0},
			{ocistorage.DeadlineExceeded, 0}, {ocistorage.Canceled, 0},
			{ocistorage.InvalidConfiguration, 0}, {ocistorage.InvalidRequest, 0},
			{ocistorage.DeadlineRequired, 0}, {ocistorage.Conflict, 412},
			{ocistorage.InvalidResponse, 200}, {ocistorage.TooLarge, 200},
			{ocistorage.Service, 404}, {ocistorage.Service, 429}, {ocistorage.Service, 503},
		} {
			t.Run(fmt.Sprintf("%s/%s/%d", stage, tc.code, tc.status), func(t *testing.T) {
				err := fmt.Errorf("private URL https://private.example: %w", &ocistorage.Error{Code: tc.code, Status: tc.status})
				got := observePinnedFailure(t, stage, err, nil)
				want := map[string]any{"category": string(tc.code)}
				if tc.status != 0 {
					want["httpStatus"] = float64(tc.status)
				}
				if !reflect.DeepEqual(got, want) {
					t.Fatalf("diagnostic=%v want=%v", got, want)
				}
			})
		}
	}
}

func TestPinnedReadLogDistinguishesIntegrityAndSize(t *testing.T) {
	for _, stage := range []string{"catalog", "configuration"} {
		for _, tc := range []struct{ category, body string }{
			{"content_mismatch", "private changed bytes"},
			{"too_large", strings.Repeat("x", (1<<20)+1)},
		} {
			t.Run(stage+"/"+tc.category, func(t *testing.T) {
				got := observePinnedFailure(t, stage, nil, []byte(tc.body))
				if !reflect.DeepEqual(got, map[string]any{"category": tc.category}) {
					t.Fatal("incorrect content failure diagnostic", got)
				}
			})
		}
	}
}
