package reconciliation

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func selectedSetupFor(t *testing.T, f *setupRuntimeFixture) (*setupCurrentStore, storedSetupCurrent) {
	t.Helper()
	s, err := newSetupCurrentStore(f.objects, f.lease.manager.environmentID, "installation-1")
	if err != nil {
		t.Fatal(err)
	}
	h, _, err := s.load(leaseBudget(t))
	if err != nil {
		t.Fatal(err)
	}
	return s, h
}

func TestSetupReadinessRejectsMissingOrAlteredEvidence(t *testing.T) {
	for _, kind := range []string{"head", "confirmation", "reference-hash", "diagnostic", "operator", "diagnostic-digest", "operator-digest", "wrong-request", "early-time", "future-time"} {
		t.Run(kind, func(t *testing.T) {
			f := setupConfirmedRuntime(t)
			v := setupReadinessFor(t, f)
			s, h := selectedSetupFor(t, f)
			cs, _ := NewSetupConfirmationStore(f.objects, s.environment, s.installation)
			ref := h.record.data.Confirmation
			confirmation, _, err := cs.load(leaseBudget(t), ref.ID)
			if err != nil {
				t.Fatal(err)
			}
			switch kind {
			case "head":
				delete(f.objects.objects, s.name())
			case "confirmation":
				delete(f.objects.objects, cs.name(ref.ID))
			case "reference-hash":
				ref.SHA256 = strings.Repeat("f", 64)
			case "diagnostic":
				delete(f.objects.objects, "control/diagnostics/installation-1/"+h.record.data.DiagnosticRequestID+".json")
			case "operator":
				delete(f.objects.objects, "control/operator-challenges/installation-1/"+h.record.data.OperatorRequestID+".json")
			case "diagnostic-digest":
				confirmation.data.DiagnosticSHA256 = strings.Repeat("f", 64)
			case "operator-digest":
				confirmation.data.OperatorSHA256 = strings.Repeat("f", 64)
			case "wrong-request":
				confirmation.data.OperatorRequestID = "other-request"
			case "early-time":
				confirmation.data.ConfirmedAt = time.Now().Add(-time.Hour).UTC().Format(leaseTimeFormat)
			case "future-time":
				confirmation.data.ConfirmedAt = time.Now().Add(time.Hour).UTC().Format(leaseTimeFormat)
			}
			if kind != "head" && kind != "confirmation" && kind != "diagnostic" && kind != "operator" {
				if kind != "reference-hash" {
					body, _ := confirmation.Encode()
					ref.ID, ref.SHA256 = confirmation.ID(), diagnosticDigest(string(body))
					f.objects.seed(cs.name(ref.ID), body)
				}
				body, _ := h.record.encode()
				f.objects.seed(s.name(), body)
			}
			f.objects.calls = nil
			if err := v.verify(leaseBudget(t), f.lease, strings.Repeat("b", 64)); err == nil {
				t.Fatal("invalid evidence enabled delivery")
			}
			for _, call := range f.objects.calls {
				if call.method != "GET" {
					t.Fatal("readiness repaired evidence", call)
				}
			}
		})
	}
}

func TestSetupReadinessRejectsCurrentChannelAndSelectionChanges(t *testing.T) {
	for _, kind := range []string{"channel-changed", "channel-error", "head-version", "head-read-error", "configuration-changed", "channel-canceled"} {
		t.Run(kind, func(t *testing.T) {
			f := setupConfirmedRuntime(t)
			v := setupReadinessFor(t, f)
			s, h := selectedSetupFor(t, f)
			ctx, cancel := context.WithCancel(leaseBudget(t))
			defer cancel()
			v.read = func(context.Context) (OperatorChannel, error) {
				channel := operatorChannelFixture()
				switch kind {
				case "channel-changed":
					channel.Endpoint = "changed@example.invalid"
				case "channel-error":
					return OperatorChannel{}, errors.New("private provider response")
				case "head-version":
					body, _ := h.record.encode()
					f.objects.seed(s.name(), body)
				case "head-read-error":
					delete(f.objects.objects, s.name())
				case "configuration-changed":
					f.objects.seed("configuration/router.json", []byte("changed"))
				case "channel-canceled":
					cancel()
				}
				return channel, nil
			}
			got, err := v.verifyOperator(ctx, f.lease)
			if err == nil || got.Record().Snapshot().State != "" || strings.Contains(err.Error(), "private provider") {
				t.Fatal("changed authority accepted or leaked", err)
			}
			if kind == "channel-canceled" && !errors.Is(err, context.Canceled) {
				t.Fatal("cancellation lost", err)
			}
		})
	}
}
