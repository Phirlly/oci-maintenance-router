package reconciliation

import (
	"context"
	"oci-maintenance-router/internal/ocinotifications"
)

// All receipt consumers observe the same concrete ACTIVE channel contract.
// Capture desired identity rather than reading later mutable configuration.
func operatorChannelReader(client *ocinotifications.Client, desired OperatorChannel) func(context.Context) (OperatorChannel, error) {
	return func(ctx context.Context) (OperatorChannel, error) {
		got, err := client.GetChannel(ctx, desired.TopicID, desired.SubscriptionID)
		if err != nil {
			return OperatorChannel{}, err
		}
		s := got.Snapshot()
		return OperatorChannel{s.TopicID, s.SubscriptionID, s.Protocol, s.Endpoint}, nil
	}
}
