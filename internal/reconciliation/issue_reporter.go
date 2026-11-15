package reconciliation

import (
	"context"
	"encoding/json"

	"oci-maintenance-router/internal/configuration"
	"oci-maintenance-router/internal/ocinotifications"
)

// IssueReporter publishes saved operational evidence. It cannot write notice
// records, retrieve credentials or authorize a PagerDuty attempt.
type IssueReporter struct {
	objects                            PreparationObjects
	environment, installation, channel string
	desired                            OperatorChannel
	controls                           currentControls
	proof                              *OperatorReceiptVerifier
	read                               func(context.Context) (operatorObservation, error)
	token                              func() string
}

type ReportingResult struct {
	Issues, Reviewed, Remaining, Deferred int
	Known, Pending, Dispatched, Accepted  bool
}

func NewIssueReporter(configBytes, catalogBytes []byte, location configuration.LedgerLocation, objects PreparationObjects, client *ocinotifications.Client) (*IssueReporter, error) {
	if objects == nil || client == nil {
		return nil, &ReportingError{"invalid_request"}
	}
	c, err := readOperatorControls(configBytes, catalogBytes, location, objects)
	if err != nil || c.mode != "LIVE" {
		return nil, &ReportingError{"live_required"}
	}
	proof, err := NewOperatorReceiptVerifier(configBytes, catalogBytes, location, objects, client)
	if err != nil {
		return nil, &ReportingError{"receipt_required"}
	}
	identity, _ := json.Marshal([]string{c.intent.TopicID, c.intent.SubscriptionID, c.intent.Protocol, c.intent.EndpointDigest})
	d := &IssueReporter{objects: objects, environment: c.intent.EnvironmentID, installation: c.intent.InstallationID,
		channel: operatorDigest(string(identity)), desired: c.desired, controls: c.current, proof: proof, token: newLeaseToken}
	d.read = func(ctx context.Context) (operatorObservation, error) {
		channel, err := client.GetChannel(ctx, d.desired.TopicID, d.desired.SubscriptionID)
		if err != nil {
			return operatorObservation{}, err
		}
		s := channel.Snapshot()
		return operatorObservation{OperatorChannel{s.TopicID, s.SubscriptionID, s.Protocol, s.Endpoint},
			func(ctx context.Context, body []byte) (ocinotifications.Publication, error) {
				return client.Publish(ctx, channel, body)
			}}, nil
	}
	return d, nil
}

func (d *IssueReporter) Retry(ctx context.Context, lease *Lease) (ReportingResult, error) {
	return d.run(ctx, lease, nil)
}

func (d *IssueReporter) Refresh(ctx context.Context, lease *Lease, snapshot ProcessingSnapshot) (ReportingResult, error) {
	return d.run(ctx, lease, &snapshot)
}
