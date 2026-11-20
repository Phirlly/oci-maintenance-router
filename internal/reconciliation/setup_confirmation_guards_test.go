package reconciliation

import (
	"context"
	"errors"
	"oci-maintenance-router/internal/ocistorage"
	"strings"
	"testing"
	"time"
)

func TestSetupConfirmationCannotCompleteAcrossStorageOrControlChanges(t *testing.T) {
	for _, kind := range []string{"confirmation-write", "head-reload", "head-changed", "final-configuration", "clock"} {
		t.Run(kind, func(t *testing.T) {
			f := setupRuntimeFixtureFor(t)
			var err error
			f.result, err = f.coordinator.Run(leaseBudget(t), f.lease)
			if err != nil {
				t.Fatal(err)
			}
			f.configure(t, "CONFIRM", 1)
			attached := false
			f.objects.before = func(_ context.Context, c discoveryCall) error {
				if kind == "confirmation-write" && c.method == "CREATE" && strings.HasPrefix(c.name, "control/setup-confirmations/") {
					return &ocistorage.Error{Code: ocistorage.Service, Status: 403}
				}
				if kind == "head-reload" && attached && c.method == "GET" && strings.HasPrefix(c.name, "control/setup-current/") {
					return errors.New("private read failure")
				}
				return nil
			}
			f.objects.after = func(_ context.Context, c discoveryCall) error {
				if c.method == "REPLACE" && strings.HasPrefix(c.name, "control/setup-current/") {
					attached = true
					switch kind {
					case "head-changed":
						d, o := f.result.Diagnostic.Record().Snapshot().Intent, f.result.Operator.Record().Snapshot().Intent
						d.RequestID, o.RequestID = setupRequestIDs(d.EnvironmentID, d.InstallationID, 2)
						next, failure := newSetupCurrent(2, d, o)
						if failure != nil {
							t.Fatal(failure)
						}
						body, _ := next.encode()
						f.objects.seed(c.name, body)
					case "clock":
						f.lease.manager.now = func() time.Time { return time.Now().Add(-time.Hour) }
					}
				}
				if kind == "final-configuration" && attached && c.method == "GET" && strings.HasPrefix(c.name, "control/setup-current/") {
					f.objects.seed("configuration/router.json", []byte("changed"))
				}
				return nil
			}
			got, err := f.coordinator.Run(leaseBudget(t), f.lease)
			if err == nil || got.Confirmation.Confirmation.ID() != "" || f.diagnosticSends != 1 || f.operatorSends != 1 {
				t.Fatal("failed final confirmation granted authority", err)
			}
		})
	}
}
