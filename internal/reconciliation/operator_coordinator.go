package reconciliation

import (
	"bytes"
	"context"
	"crypto/sha256"
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
	invalid := &OperatorRunError{Code: "invalid_request"}
	if client == nil {
		return nil, invalid
	}
	configBytes, catalogBytes = bytes.Clone(configBytes), bytes.Clone(catalogBytes)
	catalog, err := configuration.DecodeCatalog(catalogBytes)
	if err != nil {
		return nil, invalid
	}
	c, err := configuration.Decode(configBytes, catalog)
	if err != nil || c.SchemaVersion != 5 {
		return nil, invalid
	}
	if c.Mode != "TEST" || c.OperatorTest == nil {
		return nil, &OperatorRunError{Code: "intent_required"}
	}
	environment, err := c.EnvironmentID(location)
	if err != nil {
		return nil, invalid
	}
	if _, err := NewOperatorChallengeStore(objects, environment, c.InstallationID); err != nil {
		return nil, invalid
	}
	desired := OperatorChannel{c.OperatorTopicID, c.OperatorTest.SubscriptionID, "EMAIL", c.OperatorTest.Endpoint}
	intent, _ := NewOperatorIntent(environment, c.InstallationID, c.OperatorTest.RequestID, desired) // Validated by configuration.
	return &OperatorCoordinator{objects: objects, intent: intent, desired: desired,
		controls: currentControls{objects, sha256.Sum256(configBytes), sha256.Sum256(catalogBytes)},
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
