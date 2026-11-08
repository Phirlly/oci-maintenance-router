package delivery

import (
	"slices"

	"oci-maintenance-router/internal/notices"
	"oci-maintenance-router/internal/routing"
)

// Private v1 wire schema: every field is explicit, including absent timestamps.
// No caller receives its mutable collections or pointers.
type recordData struct {
	SchemaVersion int          `json:"schemaVersion"`
	EnvironmentID string       `json:"environmentId"`
	NoticeID      string       `json:"noticeId"`
	Batches       []savedBatch `json:"batches"`
}

type savedMessage struct {
	BatchID           string          `json:"batchId"`
	RevisionDigest    string          `json:"revisionDigest"`
	RoutingGeneration string          `json:"routingGeneration"`
	DestinationID     string          `json:"destinationId"`
	SecretID          string          `json:"secretId"`
	PagerDutyRegion   string          `json:"pagerDutyRegion"`
	Resources         []savedResource `json:"resources"`
	DedupKey          string          `json:"dedupKey"`
	Payload           string          `json:"payloadWithoutRoutingKey"`
	PayloadDigest     string          `json:"payloadDigest"`
}

type savedResource struct {
	ResourceID string `json:"resourceId"`
	Region     string `json:"region"`
}

type savedBatch struct {
	Message          savedMessage `json:"message"`
	SecretVersion    int64        `json:"secretVersion"`
	State            State        `json:"state"`
	AttemptCount     uint64       `json:"attemptCount"`
	OwnerToken       string       `json:"ownerToken"`
	FirstAttemptAt   *string      `json:"firstAttemptAt"`
	LastAttemptAt    *string      `json:"lastAttemptAt"`
	AttemptExpiresAt *string      `json:"attemptExpiresAt"`
	UpdatedAt        *string      `json:"updatedAt"`
	NextAttemptAt    *string      `json:"nextAttemptAt"`
	AcceptedAt       *string      `json:"acceptedAt"`
	Reason           Reason       `json:"reason"`
	Ambiguous        bool         `json:"ambiguous"`
}

func saveMessage(s Snapshot) savedMessage {
	resources := make([]savedResource, len(s.Resources))
	for i, r := range s.Resources {
		resources[i] = savedResource{r.ResourceID, r.Region}
	}
	return savedMessage{s.BatchID, s.Revision.Digest, s.RoutingGeneration, s.DestinationID,
		s.SecretID, s.PagerDutyRegion, resources, s.DedupKey, string(s.PayloadWithoutRoutingKey), s.PayloadDigest}
}

func (d *recordData) snapshot(b savedBatch) BatchSnapshot {
	m := b.Message
	resources := make([]routing.Target, len(m.Resources))
	for i, r := range m.Resources {
		resources[i] = routing.Target{ResourceID: r.ResourceID, Region: r.Region}
	}
	return BatchSnapshot{
		Message: Snapshot{BatchID: m.BatchID, EnvironmentID: d.EnvironmentID, Revision: notices.RevisionKey{NoticeID: d.NoticeID, Digest: m.RevisionDigest},
			RoutingGeneration: m.RoutingGeneration, DestinationID: m.DestinationID, SecretID: m.SecretID, PagerDutyRegion: m.PagerDutyRegion,
			Resources: resources, DedupKey: m.DedupKey, PayloadWithoutRoutingKey: []byte(m.Payload), PayloadDigest: m.PayloadDigest},
		SecretVersion: b.SecretVersion, State: b.State, AttemptCount: b.AttemptCount, OwnerToken: b.OwnerToken,
		FirstAttemptAt: savedTime(b.FirstAttemptAt), LastAttemptAt: savedTime(b.LastAttemptAt), AttemptExpiresAt: savedTime(b.AttemptExpiresAt),
		UpdatedAt: savedTime(b.UpdatedAt), NextAttemptAt: savedTime(b.NextAttemptAt), AcceptedAt: savedTime(b.AcceptedAt), Reason: b.Reason, Ambiguous: b.Ambiguous,
	}
}

func (r Record) Batches() []BatchSnapshot {
	if r.data == nil {
		return nil
	}
	result := make([]BatchSnapshot, len(r.data.Batches))
	for i, b := range r.data.Batches {
		result[i] = r.data.snapshot(b)
	}
	return result
}

func (r Record) Batch(id string) (BatchSnapshot, bool) {
	i, err := r.index(id)
	if err != nil {
		return BatchSnapshot{}, false
	}
	return r.data.snapshot(r.data.Batches[i]), true
}

func (r Record) index(id string) (int, error) {
	if r.data == nil {
		return 0, &Error{"record", "invalid"}
	}
	for i, b := range r.data.Batches {
		if b.Message.BatchID == id {
			return i, nil
		}
	}
	return 0, &Error{"batch", "unknown"}
}

func (r Record) copy() recordData {
	d := *r.data
	d.Batches = slices.Clone(d.Batches)
	// Messages, strings and time pointers are immutable; transitions replace
	// fields rather than mutating shared referents.
	return d
}
