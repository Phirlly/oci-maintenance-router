package reconciliation

import (
	"context"
	"oci-maintenance-router/internal/configuration"
	"oci-maintenance-router/internal/ocinotifications"
)

type operatorObservation struct {
	channel OperatorChannel
	publish func(context.Context, []byte) (ocinotifications.Publication, error)
}

// OperatorCoordinator owns one pinned TEST intent. Bootstrap binds the client,
// object backend and lease to the trusted region, principal and ledger location.
// It does not establish human receipt or permission to activate LIVE processing.
type OperatorCoordinator struct {
	objects  OperatorChallengeObjects
	intent   OperatorIntent
	desired  OperatorChannel
	controls currentControls
	read     func(context.Context) (operatorObservation, error)
	token    func() string
}

// OperatorResult distinguishes observed publication from confirmed saved state.
// Dispatched means the adapter was called; a status alone is not acceptance.
// Neither API_ACCEPTED nor a message ID proves receipt. Existing records have
// no new dispatch, observed status or observed message ID.
type OperatorResult struct {
	record     OperatorChallenge
	dispatched bool
	status     int
	messageID  string
}

func (r OperatorResult) Record() OperatorChallenge { return r.record }
func (r OperatorResult) Dispatched() bool          { return r.dispatched }
func (r OperatorResult) ObservedStatus() int       { return r.status }
func (r OperatorResult) ObservedMessageID() string { return r.messageID }

func NewOperatorCoordinator(configBytes, catalogBytes []byte, location configuration.LedgerLocation, objects OperatorChallengeObjects, client *ocinotifications.Client) (*OperatorCoordinator, error) {
	if client == nil {
		return nil, &OperatorRunError{Code: "invalid_request"}
	}
	c, err := readOperatorControls(configBytes, catalogBytes, location, objects)
	if err != nil {
		return nil, err
	}
	if c.mode != "TEST" {
		return nil, &OperatorRunError{Code: "intent_required"}
	}
	desired := c.desired
	return &OperatorCoordinator{objects: objects, intent: c.intent, desired: desired,
		controls: c.current,
		read: func(ctx context.Context) (operatorObservation, error) {
			channel, err := client.GetChannel(ctx, desired.TopicID, desired.SubscriptionID)
			if err != nil {
				return operatorObservation{}, err
			}
			s := channel.Snapshot()
			return operatorObservation{OperatorChannel{s.TopicID, s.SubscriptionID, s.Protocol, s.Endpoint},
				func(ctx context.Context, payload []byte) (ocinotifications.Publication, error) {
					return client.Publish(ctx, channel, payload)
				}}, nil
		}, token: newLeaseToken}, nil
}
