package deployment

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"oci-maintenance-router/internal/ocistorage"
)

func TestPinnedReadLogRedactsUnknownErrors(t *testing.T) {
	var nilStorageError *ocistorage.Error
	for _, err := range []error{
		errors.New("private credential and object data"),
		&ocistorage.Error{Code: "private-category", Status: 503},
		nilStorageError,
		fmt.Errorf("private wrapper: %w", nilStorageError),
	} {
		got := observePinnedFailure(t, "catalog", err, nil)
		if !reflect.DeepEqual(got, map[string]any{"category": "unknown"}) {
			t.Fatal("unrecognized error escaped", got)
		}
	}
}

func TestPinnedReadLogLimitsHTTPStatus(t *testing.T) {
	for _, status := range []int{-1, 0, 99, 100, 599, 600, 12345678} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			got := observePinnedFailure(t, "configuration", &ocistorage.Error{Code: ocistorage.Service, Status: status}, nil)
			want := map[string]any{"category": "service"}
			if status >= 100 && status <= 599 {
				want["httpStatus"] = float64(status)
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatal("unbounded HTTP status", got)
			}
		})
	}
}

func TestPinnedReadLogOmittedOutsidePinnedReadFailure(t *testing.T) {
	for _, kind := range []string{"success", "bootstrap", "input", "catalog-json", "configuration-json", "write"} {
		t.Run(kind, func(t *testing.T) {
			h, settings, store := handlerFixture(t)
			input := invocation(settings)
			switch kind {
			case "bootstrap":
				h.getenv = func(string) string { return "invalid" }
			case "input":
				input = "invalid"
			case "catalog-json":
				store.catalog = []byte("invalid")
				settings.CatalogSHA256 = digestFixture(store.catalog)
			case "configuration-json":
				store.config = []byte("invalid")
				settings.ConfigurationSHA256 = digestFixture(store.config)
			case "write":
				store.createError = errors.New("private write failure")
			}
			if kind == "catalog-json" || kind == "configuration-json" {
				raw, _ := json.Marshal(settings)
				h.getenv = func(string) string { return string(raw) }
				input = invocation(settings)
			}
			var logs bytes.Buffer
			h.logger = log.New(&logs, "", 0)
			h.Serve(testContext(t), strings.NewReader(input), httptest.NewRecorder())
			var got map[string]any
			if json.Unmarshal(logs.Bytes(), &got) != nil || got["pinnedRead"] != nil || strings.Contains(logs.String(), "private") {
				t.Fatal("diagnostic added outside pinned read failure", logs.String())
			}
			if kind == "success" {
				if got["status"] != "deployment-verified" || got["code"] != nil {
					t.Fatal("success fixture did not complete", got)
				}
			} else {
				code := map[string]string{"bootstrap": "bootstrap_invalid", "input": "request_invalid", "catalog-json": "catalog_invalid", "configuration-json": "configuration_invalid", "write": "storage_write"}[kind]
				if got["status"] != "not-ready" || got["code"] != code {
					t.Fatal("fixture did not reach intended failure", got)
				}
			}
		})
	}
}
