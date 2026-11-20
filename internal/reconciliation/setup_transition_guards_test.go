package reconciliation

import (
	"context"
	"errors"
	"oci-maintenance-router/internal/ocistorage"
	"strings"
	"testing"
	"time"
)

func TestSetupSelectionStopsAtEveryConfigurationChangeBoundary(t *testing.T) {
	for _, stage := range []string{"after-create", "before-replace", "after-diagnostic", "before-history"} {
		t.Run(stage, func(t *testing.T) {
			f := setupRuntimeFixtureFor(t)
			if stage == "before-replace" || stage == "after-diagnostic" {
				if _, err := f.coordinator.Run(leaseBudget(t), f.lease); err != nil {
					t.Fatal(err)
				}
				if stage == "before-replace" {
					f.configure(t, "SEND_TEST", 2)
				}
			}
			beforeD, beforeO := f.diagnosticSends, f.operatorSends
			headReads := 0
			f.objects.before = func(_ context.Context, c discoveryCall) error {
				if stage == "before-history" && c.method == "GET" && strings.HasPrefix(c.name, "control/setup-current/") {
					headReads++
					if headReads == 2 {
						f.lease.manager.now = func() time.Time { return f.lease.deadline.Add(-time.Second) }
					}
				}
				return nil
			}
			f.objects.after = func(_ context.Context, c discoveryCall) error {
				if stage == "after-create" && c.method == "CREATE" && strings.HasPrefix(c.name, "control/setup-current/") || stage == "before-replace" && c.method == "GET" && strings.HasPrefix(c.name, "control/setup-current/") || stage == "after-diagnostic" && c.method == "GET" && strings.HasPrefix(c.name, "control/diagnostics/") {
					f.objects.seed("configuration/router.json", []byte("changed"))
				}
				return nil
			}
			if _, err := f.coordinator.Run(leaseBudget(t), f.lease); err == nil {
				t.Fatal("changed authority completed setup")
			}
			if f.diagnosticSends != beforeD || f.operatorSends != beforeO {
				t.Fatal("authority change sent a channel")
			}
			if stage == "before-replace" && preparationCalls(f.objects, "REPLACE", "control/setup-current/installation-1.json") != 0 {
				t.Fatal("changed configuration advanced selection")
			}
		})
	}
}

func TestSetupCurrentUnreadableLostWriteNeverReturnsHandle(t *testing.T) {
	s, objects, r := setupCurrentStoreFixture(t)
	written := false
	objects.after = func(_ context.Context, c discoveryCall) error {
		if c.method == "CREATE" && c.name == s.name() {
			written = true
			return &ocistorage.Error{Code: ocistorage.Transport, WriteUncertain: true}
		}
		return nil
	}
	objects.before = func(_ context.Context, c discoveryCall) error {
		if written && c.method == "GET" && c.name == s.name() {
			return &ocistorage.Error{Code: ocistorage.Service, Status: 403}
		}
		return nil
	}
	got, err := s.create(leaseBudget(t), r)
	var detail *SetupError
	if !errors.As(err, &detail) || !detail.WriteUncertain || detail.HTTPStatus != 403 || got.record.data != nil || preparationCalls(objects, "CREATE", s.name()) != 1 {
		t.Fatal("unreadable uncertain write became authority", err)
	}
}
