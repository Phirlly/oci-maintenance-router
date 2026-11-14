package reconciliation

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"oci-maintenance-router/internal/ocistorage"
)

func TestPreparationRefusesMissingCorruptOrMismatchedHistory(t *testing.T) {
	for _, kind := range []string{"head-corrupt", "head-foreign", "head-read", "baseline-missing", "baseline-corrupt", "baseline-changed"} {
		t.Run(kind, func(t *testing.T) {
			p, s, l := preparationFixture(t, nil)
			if err := p.Initialize(leaseBudget(t), l); err != nil {
				t.Fatal(err)
			}
			b := prepareSuccessfully(t, p, l)
			name := "baselines/" + b.ID() + ".json"
			code := "invalid_record"
			switch kind {
			case "head-corrupt":
				s.seed(preparedHistoryName, []byte("private record"))
			case "head-foreign":
				h, _ := NewPreparedHistory(l.manager.environmentID, "other-installation")
				body, _ := h.Encode()
				s.seed(preparedHistoryName, body)
			case "head-read":
				code = "read_failed"
				s.before = func(_ context.Context, c discoveryCall) error {
					if c.name == preparedHistoryName {
						return &ocistorage.Error{Code: ocistorage.Service, Status: 403}
					}
					return nil
				}
			case "baseline-missing":
				delete(s.objects, name)
				code = "read_failed"
			case "baseline-corrupt":
				s.seed(name, []byte("private baseline"))
			case "baseline-changed":
				body, _ := b.Encode()
				body = bytes.ReplaceAll(body, []byte(baselineCompartment), []byte(baselineOtherCompartment))
				s.seed(name, body)
				code = "history_mismatch"
			}
			before := preparationCalls(s, "CREATE", "") + preparationCalls(s, "REPLACE", "")
			got, err := p.Prepare(leaseBudget(t), l)
			preparationFailed(t, got, err, code)
			if preparationCalls(s, "CREATE", "")+preparationCalls(s, "REPLACE", "") != before {
				t.Fatal("broken history caused replacement")
			}
			if kind == "head-corrupt" || kind == "head-foreign" || kind == "head-read" {
				err = p.Initialize(leaseBudget(t), l)
				preparationFailed(t, Baseline{}, err, code)
				if preparationCalls(s, "HISTORY", "") != 1 {
					t.Fatal("unreadable history treated as first initialization")
				}
			}
		})
	}
}

func TestPreparationRejectsTargetWithWrongCellsOrAncestry(t *testing.T) {
	for _, kind := range []string{"cells", "prefix", "layers", "malformed", "identity"} {
		t.Run(kind, func(t *testing.T) {
			p, s, l := preparationFixture(t, nil)
			if err := p.Initialize(leaseBudget(t), l); err != nil {
				t.Fatal(err)
			}
			if kind == "prefix" {
				prepareSuccessfully(t, p, l)
				p = preparationFor(t, s, preparationConfig(t, func(c map[string]any) { c["compartmentIds"] = []string{baselineCompartment, baselineOtherCompartment} }), nil)
			}
			s.before = func(_ context.Context, c discoveryCall) error {
				if c.method == "REPLACE" && c.name == preparedHistoryName {
					return &ocistorage.Error{Code: ocistorage.Service, Status: 503}
				}
				return nil
			}
			got, err := p.Prepare(leaseBudget(t), l)
			preparationFailed(t, got, err, "write_failed")
			s.before = nil
			h := preparationHead(t, s, l)
			id := p.targetID(h)
			name := "baselines/" + id + ".json"
			var wire map[string]any
			if err := json.Unmarshal(s.objects[name].Body, &wire); err != nil {
				t.Fatal(err)
			}
			layers := wire["layers"].([]any)
			first := layers[0].(map[string]any)
			code := "target_mismatch"
			switch kind {
			case "cells":
				first["cells"].([]any)[0].(map[string]any)["compartmentId"] = baselineOtherCompartment
			case "prefix":
				first["completedAt"] = "2026-11-12T12:00:00.000Z"
			case "layers":
				wire["layers"] = append(layers, map[string]any{"completedAt": first["completedAt"], "cells": []any{map[string]any{"region": "us-ashburn-1", "compartmentId": baselineOtherCompartment}}, "revisions": []any{}})
			case "identity":
				wire["baselineId"] = "wrong-id"
				code = "invalid_record"
			case "malformed":
				code = "invalid_record"
			}
			body, _ := json.Marshal(wire)
			// Re-encode a valid private model to preserve the canonical wire contract.
			if kind != "identity" && kind != "malformed" {
				var data baselineData
				if err := json.Unmarshal(body, &data); err != nil {
					t.Fatal(err)
				}
				body, _ = json.Marshal(data)
			}
			if kind == "malformed" {
				body = []byte("private target")
			}
			s.seed(name, body)
			before := preparationCalls(s, "LIST", "")
			got, err = p.Prepare(leaseBudget(t), l)
			preparationFailed(t, got, err, code)
			if preparationCalls(s, "LIST", "") != before || preparationHead(t, s, l).BaselineID() != h.BaselineID() {
				t.Fatal("bad target was rebuilt or selected")
			}
		})
	}
}
