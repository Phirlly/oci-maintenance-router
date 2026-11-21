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
	setupProof                         *setupReadiness
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
	catalog, err := configuration.DecodeCatalog(catalogBytes)
	if err != nil {
		return nil, &ReportingError{"live_required"}
	}
	config, err := configuration.Decode(configBytes, catalog)
	if err != nil || config.Mode != "LIVE" {
		return nil, &ReportingError{"live_required"}
	}
	d := &IssueReporter{objects: objects, token: newLeaseToken}
	if config.SchemaVersion == 8 || config.SchemaVersion == 9 {
		v, err := newSetupReadiness(configBytes, catalogBytes, location, objects, client)
		if err != nil {
			return nil, &ReportingError{"receipt_required"}
		}
		d.environment, d.installation, d.desired, d.controls, d.setupProof = v.input.environment, v.input.config.InstallationID, v.input.desired, v.input.current, v
	} else {
		c, err := readOperatorControls(configBytes, catalogBytes, location, objects)
		if err != nil {
			return nil, &ReportingError{"live_required"}
		}
		proof, err := NewOperatorReceiptVerifier(configBytes, catalogBytes, location, objects, client)
		if err != nil {
			return nil, &ReportingError{"receipt_required"}
		}
		d.environment, d.installation, d.desired, d.controls, d.proof = c.intent.EnvironmentID, c.intent.InstallationID, c.desired, c.current, proof
	}
	target, _ := json.Marshal([]string{"operator-target-v1", d.desired.Protocol, d.desired.Endpoint})
	identity, _ := json.Marshal([]string{d.desired.TopicID, d.desired.SubscriptionID, d.desired.Protocol, operatorDigest(string(target))})
	d.channel = operatorDigest(string(identity))
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

func (d *IssueReporter) verifyReceipt(ctx context.Context, lease *Lease) error {
	if d.setupProof != nil {
		_, err := d.setupProof.verifyOperator(ctx, lease)
		return err
	}
	_, err := d.proof.Verify(ctx, lease)
	return err
}

func (d *IssueReporter) Retry(ctx context.Context, lease *Lease) (ReportingResult, error) {
	return d.run(ctx, lease, nil)
}

func (d *IssueReporter) Refresh(ctx context.Context, lease *Lease, snapshot ProcessingSnapshot) (ReportingResult, error) {
	return d.run(ctx, lease, &snapshot)
}
