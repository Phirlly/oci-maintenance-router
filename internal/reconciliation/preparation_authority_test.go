package reconciliation

import (
	"context"
	"strings"
	"testing"

	"oci-maintenance-router/internal/ocistorage"
)

func TestPreparationRechecksControlsAfterEverySuccessfulPath(t *testing.T) {
	for _, stage := range []string{"existing-init", "init-write", "init-confirm", "covered", "reused-target", "baseline-write", "baseline-confirm", "head-write", "head-confirm"} {
		for _, control := range []string{"config", "catalog", "lease"} {
			t.Run(stage+"/"+control, func(t *testing.T) {
				p, s, l := preparationFixture(t, nil)
				initialize := strings.Contains(stage, "init")
				if stage != "init-write" && stage != "init-confirm" {
					if err := p.Initialize(leaseBudget(t), l); err != nil {
						t.Fatal(err)
					}
				}
				if stage == "covered" {
					prepareSuccessfully(t, p, l)
				}
				if stage == "reused-target" {
					s.before = func(_ context.Context, c discoveryCall) error {
						if c.method == "REPLACE" && c.name == preparedHistoryName {
							return &ocistorage.Error{Code: ocistorage.Service, Status: 503}
						}
						return nil
					}
					b, err := p.Prepare(leaseBudget(t), l)
					preparationFailed(t, b, err, "write_failed")
					s.before = nil
				}
				changed := false
				confirmPending := false
				change := func() {
					changed = true
					switch control {
					case "config":
						s.seed("configuration/router.json", []byte("changed"))
					case "catalog":
						s.seed("configuration/catalog.json", []byte("changed"))
					case "lease":
						s.seed(workerLeaseName, l.record.bytes())
					}
				}
				s.after = func(_ context.Context, c discoveryCall) error {
					if changed {
						return nil
					}
					if confirmPending && c.method == "GET" && (c.name == preparedHistoryName || strings.HasPrefix(c.name, "baselines/")) {
						change()
						return nil
					}
					match := stage == "existing-init" && c.method == "GET" && c.name == preparedHistoryName || stage == "covered" && c.method == "GET" && strings.HasPrefix(c.name, "baselines/") || stage == "reused-target" && c.method == "GET" && strings.HasPrefix(c.name, "baselines/") || strings.HasPrefix(stage, "init-") && c.method == "CREATE" && c.name == preparedHistoryName || strings.HasPrefix(stage, "baseline-") && c.method == "CREATE" && strings.HasPrefix(c.name, "baselines/") || strings.HasPrefix(stage, "head-") && c.method == "REPLACE" && c.name == preparedHistoryName
					if match {
						if strings.HasSuffix(stage, "confirm") {
							confirmPending = true
							return &ocistorage.Error{Code: ocistorage.Transport, WriteUncertain: true}
						}
						change()
					}
					return nil
				}
				var b Baseline
				var err error
				if initialize {
					err = p.Initialize(leaseBudget(t), l)
				} else {
					b, err = p.Prepare(leaseBudget(t), l)
				}
				code := "configuration_changed"
				if control == "lease" {
					code = "ownership_lost"
				}
				preparationFailed(t, b, err, code)
				if !changed {
					t.Fatal("authority mutation did not execute")
				}
				if strings.HasPrefix(stage, "baseline-") || stage == "reused-target" {
					if preparationHead(t, s, l).BaselineID() != "" {
						t.Fatal("lost authority still advanced head")
					}
				}
			})
		}
	}
}

func TestPreparationStopsAfterChangedControlsBeforeHistoryCreate(t *testing.T) {
	p, s, l := preparationFixture(t, nil)
	s.history = func(context.Context, string) (bool, error) {
		s.seed("configuration/router.json", []byte("changed"))
		return true, nil
	}
	err := p.Initialize(leaseBudget(t), l)
	preparationFailed(t, Baseline{}, err, "configuration_changed")
	if preparationCalls(s, "CREATE", preparedHistoryName) != 0 {
		t.Fatal("changed controls initialized history")
	}
}
