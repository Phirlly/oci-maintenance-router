package reconciliation

import (
	"context"
	"errors"
	"testing"

	"oci-maintenance-router/internal/ocinotifications"
)

func TestOperatorCoordinatorChecksBothCurrentChannelObservations(t *testing.T) {
	for _, stage := range []int32{1, 2} {
		for _, kind := range []string{"topic", "subscription", "protocol", "endpoint", "publisher", "inactive", "deleted", "raw-error", "late-success"} {
			t.Run(string(rune('0'+stage))+"/"+kind, func(t *testing.T) {
				f := operatorCoordinatorFixtureFor(t)
				ctx, cancel := context.WithCancel(leaseBudget(t))
				defer cancel()
				old := f.coordinator.read
				f.coordinator.read = func(c context.Context) (operatorObservation, error) {
					o, err := old(c)
					if f.reads.Load() != stage {
						return o, err
					}
					switch kind {
					case "topic":
						o.channel.TopicID += "different"
					case "subscription":
						o.channel.SubscriptionID += "different"
					case "protocol":
						o.channel.Protocol = "HTTPS"
					case "endpoint":
						o.channel.Endpoint = "team@example.invalid"
					case "publisher":
						o.publish = nil
					case "inactive":
						return operatorObservation{}, &ocinotifications.Error{Code: ocinotifications.InvalidResponse, Status: 200}
					case "deleted":
						return operatorObservation{}, &ocinotifications.Error{Code: ocinotifications.Service, Status: 404}
					case "raw-error":
						return o, errors.New("private target and provider")
					case "late-success":
						cancel()
					}
					return o, nil
				}
				got, err := f.coordinator.Run(ctx, f.lease)
				want := "channel_changed"
				if kind == "inactive" || kind == "deleted" || kind == "raw-error" {
					want = "channel_read"
				}
				if kind == "late-success" {
					want = "canceled"
				}
				e := operatorRunFailure(t, err, want)
				if kind == "deleted" && e.HTTPStatus != 404 {
					t.Fatal("lost controlled status")
				}
				if got.Dispatched() || f.sends.Load() != 0 || f.reads.Load() != stage {
					t.Fatal("invalid channel still published")
				}
				if stage == 1 {
					if preparationCalls(f.objects, "CREATE", operatorRunObjectName) != 0 || got.Record().Snapshot().State != "" {
						t.Fatal("unready first channel consumed request")
					}
				} else {
					if got.Record().Snapshot().State != OperatorPublishing {
						t.Fatal("consumed claim discarded")
					}
					again, err := f.coordinator.Run(leaseBudget(t), f.lease)
					if err != nil || again.Dispatched() || f.reads.Load() != stage {
						t.Fatal("consumed claim reread channel or resent", err)
					}
				}
			})
		}
	}
}

func TestOperatorCoordinatorUsesOnlySecondBoundPublisher(t *testing.T) {
	f := operatorCoordinatorFixtureFor(t)
	old := f.coordinator.read
	f.coordinator.read = func(ctx context.Context) (operatorObservation, error) {
		o, err := old(ctx)
		if f.reads.Load() == 1 {
			o.publish = func(context.Context, []byte) (ocinotifications.Publication, error) {
				t.Error("stale channel publisher used")
				return ocinotifications.Publication{}, nil
			}
		}
		return o, err
	}
	got, err := f.coordinator.Run(leaseBudget(t), f.lease)
	if err != nil || !got.Dispatched() || f.reads.Load() != 2 || f.sends.Load() != 1 {
		t.Fatal("fresh channel not used", err)
	}
}

func TestOperatorCoordinatorPendingChannelCanBeConfirmedBeforeRetry(t *testing.T) {
	f := operatorCoordinatorFixtureFor(t)
	old := f.coordinator.read
	f.coordinator.read = func(context.Context) (operatorObservation, error) {
		return operatorObservation{}, &ocinotifications.Error{Code: ocinotifications.InvalidResponse, Status: 200}
	}
	_, err := f.coordinator.Run(leaseBudget(t), f.lease)
	operatorRunFailure(t, err, "channel_read")
	f.coordinator.read = old
	got, err := f.coordinator.Run(leaseBudget(t), f.lease)
	if err != nil || got.Record().Snapshot().State != OperatorAccepted || f.sends.Load() != 1 {
		t.Fatal("unclaimed request could not continue after confirmation", err)
	}
}
