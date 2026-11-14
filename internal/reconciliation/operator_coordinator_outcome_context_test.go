package reconciliation

import (
	"context"
	"testing"
	"time"

	"oci-maintenance-router/internal/ocinotifications"
	"oci-maintenance-router/internal/ocistorage"
)

func TestOperatorCoordinatorCanceledWorkOnlyConfirmsIssuedOutcome(t *testing.T) {
	f := operatorCoordinatorFixtureFor(t)
	ctx, cancel := context.WithCancel(leaseBudget(t))
	defer cancel()
	sent := false
	var methods []string
	f.objects.before = func(c context.Context, call discoveryCall) error {
		if sent {
			if call.name != operatorRunObjectName || (call.method != "REPLACE" && call.method != "GET") || c.Err() != nil {
				t.Fatal("detached context reused beyond exact outcome")
			}
			methods = append(methods, call.method)
		}
		return nil
	}
	f.objects.after = func(_ context.Context, call discoveryCall) error {
		if sent && call.method == "REPLACE" {
			return &ocistorage.Error{Code: ocistorage.Transport, WriteUncertain: true}
		}
		return nil
	}
	f.publish = func(context.Context, []byte) (ocinotifications.Publication, error) {
		sent = true
		f.sends.Add(1)
		cancel()
		return ocinotifications.Publication{Status: 200, MessageID: "message-1"}, nil
	}
	got, err := f.coordinator.Run(ctx, f.lease)
	if err != nil || got.Record().Snapshot().State != OperatorAccepted || f.sends.Load() != 1 || len(methods) != 2 || methods[0] != "REPLACE" || methods[1] != "GET" {
		t.Fatal("exact canceled outcome confirmation failed", err)
	}
}

func TestOperatorCoordinatorCannotConfirmOutcomeBeyondReserve(t *testing.T) {
	for _, kind := range []string{"clock", "deadline"} {
		t.Run(kind, func(t *testing.T) {
			f := operatorCoordinatorFixtureFor(t)
			replaced := false
			at := f.lease.manager.now()
			f.objects.after = func(_ context.Context, c discoveryCall) error {
				if c.method == "REPLACE" && c.name == operatorRunObjectName {
					replaced = true
					if kind == "clock" {
						f.lease.manager.now = func() time.Time { return at.Add(-time.Second) }
					} else {
						f.lease.manager.now = func() time.Time { return f.lease.deadline }
					}
					return &ocistorage.Error{Code: ocistorage.Transport, WriteUncertain: true}
				}
				return nil
			}
			got, err := f.coordinator.Run(leaseBudget(t), f.lease)
			e := operatorRunFailure(t, err, "read_failed")
			if !replaced || !got.Dispatched() || got.ObservedStatus() != 200 || got.ObservedMessageID() != "message-1" || got.Record().Snapshot().State != OperatorPublishing || f.sends.Load() != 1 {
				t.Fatal("unconfirmed outcome lost observation or replaced last confirmed record")
			}
			if !e.WriteUncertain {
				t.Fatal("uncertainty lost")
			}
		})
	}
}
