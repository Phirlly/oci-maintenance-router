package reconciliation

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"oci-maintenance-router/internal/ocistorage"
)

func TestLeaseFailurePreservesRequestEvidenceWithoutExtraCalls(t *testing.T) {
	for _, tc := range []struct {
		name, request, code, category string
		status                        int
		calls                         []string
		priorCategory                 string
		priorStatus                   int
	}{
		{"create", "create", "write_failed", "service", 403, []string{"CREATE"}, "", 0},
		{"read", "read", "read_failed", "service", 404, []string{"CREATE", "GET"}, "", 0},
		{"replace", "replace", "write_failed", "service", 503, []string{"CREATE", "GET", "REPLACE"}, "", 0},
		{"confirm-create", "create_confirmation", "read_failed", "service", 404, []string{"CREATE", "GET"}, "service", 503},
		{"confirm-replace", "replace_confirmation", "ownership_lost", "", 0, []string{"CREATE", "GET", "REPLACE", "GET"}, "transport", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			now := leaseTime()
			s := &leaseMemory{}
			if tc.name == "read" || tc.name == "replace" || tc.name == "confirm-replace" {
				seedLease(s, now.Add(-601*time.Second))
			}
			var calls []string
			f := leaseFunctionsFor(s)
			f.create = func(c context.Context, n string, b []byte) (string, error) {
				calls = append(calls, "CREATE")
				if tc.name == "create" {
					return "", &ocistorage.Error{Code: ocistorage.Service, Status: 403}
				}
				if tc.name == "confirm-create" {
					return "", &ocistorage.Error{Code: ocistorage.Service, Status: 503, WriteUncertain: true}
				}
				return s.Create(c, n, b)
			}
			f.get = func(c context.Context, n string) (ocistorage.Object, error) {
				calls = append(calls, "GET")
				if tc.name == "read" || tc.name == "confirm-create" {
					return ocistorage.Object{}, &ocistorage.Error{Code: ocistorage.Service, Status: 404}
				}
				return s.Get(c, n)
			}
			f.replace = func(context.Context, string, string, []byte) (string, error) {
				calls = append(calls, "REPLACE")
				if tc.name == "confirm-replace" {
					return "", &ocistorage.Error{Code: ocistorage.Transport, WriteUncertain: true}
				}
				return "", &ocistorage.Error{Code: ocistorage.Service, Status: 503}
			}
			lease, err := leaseManager(t, f, now).Acquire(leaseBudget(t))
			leaseFailure(t, err, tc.code)
			if lease != nil || !reflect.DeepEqual(calls, tc.calls) {
				t.Fatal("ownership or request sequence changed", calls)
			}
			// Decode observational fields without requiring new production types
			// so the original implementation fails behaviorally, not at compile time.
			var got struct {
				Request, StorageCategory string
				HTTPStatus               int
				WriteUncertain           bool
				PriorWrite               *struct {
					Category   string
					HTTPStatus int
				}
			}
			raw, _ := json.Marshal(err)
			if json.Unmarshal(raw, &got) != nil || got.Request != tc.request || got.StorageCategory != tc.category || got.HTTPStatus != tc.status {
				t.Fatal("missing final request evidence", string(raw))
			}
			if tc.priorCategory == "" {
				if got.PriorWrite != nil || got.WriteUncertain {
					t.Fatal("invented uncertainty", string(raw))
				}
			} else if !got.WriteUncertain || got.PriorWrite == nil || got.PriorWrite.Category != tc.priorCategory || got.PriorWrite.HTTPStatus != tc.priorStatus {
				t.Fatal("lost initiating uncertain-write evidence", string(raw))
			}
		})
	}
}
