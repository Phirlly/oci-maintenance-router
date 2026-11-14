package reconciliation

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"strings"
	"testing"
	"time"

	"oci-maintenance-router/internal/delivery"
	"oci-maintenance-router/internal/ocisecrets"
	"oci-maintenance-router/internal/pagerduty"
	"oci-maintenance-router/internal/routing"
)

func acceptanceKey(id string, version int64) []byte {
	digest := sha256.Sum256([]byte(fmt.Sprintf("synthetic:%s:%d", id, version)))
	return []byte(fmt.Sprintf("%x", digest)[:32])
}

// Every invocation constructs its own lease, controls, stores and processor.
// Only I/O callbacks are replaced; real internal orchestration remains intact.
func (s *acceptanceService) run(t *testing.T) ProcessingSnapshot {
	t.Helper()
	manager, err := NewLeaseManager(s.objects, s.environment(t))
	if err != nil {
		t.Fatal(err)
	}
	lease := acquireLease(t, manager, leaseBudget(t))
	defer acceptanceRelease(t, lease)
	if lease.record.OwnerToken == s.previousOwner {
		t.Fatal("worker reused ownership")
	}
	s.previousOwner = lease.record.OwnerToken
	p, err := NewMaintenanceProcessor(s.config, []byte(preparationCatalog), candidateLocation(), strings.Repeat("b", 64), s.source(), s.objects, operatorClient(t), diagnosticVault(t), processingMySQL(t))
	if err != nil {
		t.Fatal(err)
	}
	p.selector.operator.read = func(ctx context.Context) (OperatorChannel, error) {
		return p.selector.operator.input.desired, ctx.Err()
	}
	p.wait = func(ctx context.Context, _ time.Duration) error { return ctx.Err() }
	p.discovery.wait, p.jitter = p.wait, func(time.Duration) time.Duration { return 0 }
	p.mysql["us-ashburn-1"] = func(ctx context.Context, id string) (routing.Metadata, error) {
		compartment := "ocid1.compartment.oc1..example"
		if s.owners[id] == "outside" {
			compartment = "ocid1.compartment.oc1..unselected"
		}
		tags := map[string]string{}
		if owner := s.owners[id]; owner != "" {
			tags["owner"] = owner
		}
		return routing.Metadata{ID: chain(id), CompartmentID: chain(compartment), FreeformTags: tags}, ctx.Err()
	}
	p.current = func(ctx context.Context, id string) (maintenanceCredential, error) {
		s.currentReads = append(s.currentReads, id)
		if id == s.blockedSecret {
			return nil, &ocisecrets.Error{Code: ocisecrets.Service, Status: 403}
		}
		version := s.currentVersion
		return diagnosticMaterial{version, func() []byte { return acceptanceKey(id, version) }}, ctx.Err()
	}
	p.version = func(ctx context.Context, id string, version int64) (maintenanceCredential, error) {
		s.pinnedReads = append(s.pinnedReads, fmt.Sprintf("%s@%d", id, version))
		return diagnosticMaterial{version, func() []byte { return acceptanceKey(id, version) }}, ctx.Err()
	}
	p.senders["US"] = func(ctx context.Context, body, key []byte) (pagerduty.Response, error) {
		if ctx.Err() != nil || !pagerduty.ValidRoutingKey(key) {
			t.Fatal("invalid dispatch lifetime or key")
		}
		for id := range s.notices {
			for _, b := range s.saved(t, id).Snapshot().Batches {
				if !bytes.Equal(body, b.Message.PayloadWithoutRoutingKey) {
					continue
				}
				if b.State != delivery.Sending || b.OwnerToken == lease.record.OwnerToken || b.Message.SecretID != "ocid1.vaultsecret.oc1.iad."+b.Message.DestinationID || !bytes.Equal(key, acceptanceKey(b.Message.SecretID, b.SecretVersion)) {
					t.Fatal("dispatch lacks distinct saved claim/exact key")
				}
				sent := acceptanceSend{destination: b.Message.DestinationID, payload: bytes.Clone(body), version: b.SecretVersion}
				for _, r := range b.Message.Resources {
					sent.resources = append(sent.resources, r.ResourceID)
				}
				s.sends = append(s.sends, sent)
				status := s.statuses[sent.destination]
				if status == 0 {
					status = 202
				}
				return pagerduty.Response{Status: status}, nil
			}
		}
		t.Fatal("payload does not match any saved batch")
		return pagerduty.Response{}, nil
	}
	p.senders["EU"] = func(context.Context, []byte, []byte) (pagerduty.Response, error) {
		t.Fatal("unconfigured region dispatched")
		return pagerduty.Response{}, nil
	}
	result, err := p.Run(leaseBudget(t), lease)
	if err != nil || !result.Snapshot().Completed {
		t.Fatal("acceptance processing", err, result.Snapshot())
	}
	return result.Snapshot()
}
