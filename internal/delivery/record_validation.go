package delivery

import "regexp"

var (
	recordNoticeID = regexp.MustCompile(`^ocid1\.announcement\.oc1\.\.([A-Za-z0-9_-]*\.)?[A-Za-z0-9_-]+$`)
	recordSecretID = regexp.MustCompile(`^ocid1\.vaultsecret\.oc1\.[a-z0-9][a-z0-9-]*\.([A-Za-z0-9_-]*\.)?[A-Za-z0-9_-]+$`)
	recordName     = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,31}$`)
	recordRegion   = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
)

func validateRecord(d *recordData) error {
	if (d.SchemaVersion != 1 && d.SchemaVersion != 2 && d.SchemaVersion != 3) || !recordDigest.MatchString(d.EnvironmentID) || !recordNoticeID.MatchString(d.NoticeID) || len(d.Batches) == 0 {
		return &Error{"record", "invalid_schema"}
	}
	previous := ""
	for _, b := range d.Batches {
		if b.Message.BatchID <= previous {
			return &Error{"batches", "invalid_order"}
		}
		previous = b.Message.BatchID
		if err := validateMessage(d, b); err != nil {
			return err
		}
		if !validAttemptKind(b, d.SchemaVersion) || !validState(b, d.SchemaVersion) {
			return &Error{"state", "invalid"}
		}
	}
	if err := validateAssignments(d.Batches); err != nil {
		return err
	}
	return recordCapacity(d)
}

func validateMessage(d *recordData, b savedBatch) error {
	m := b.Message
	if !recordDigest.MatchString(m.RevisionDigest) || !recordDigest.MatchString(m.RoutingGeneration) || !recordName.MatchString(m.DestinationID) || !recordSecretID.MatchString(m.SecretID) || b.SecretVersion <= 0 || (m.PagerDutyRegion != "US" && m.PagerDutyRegion != "EU") {
		return &Error{"message", "invalid_metadata"}
	}
	if len(m.Resources) == 0 || len(m.Resources) > maxResourcesPerDraft {
		return &Error{"resources", "invalid_count"}
	}
	previous := ""
	for _, r := range m.Resources {
		if r.ResourceID <= previous || !validResourceID(r.ResourceID, d.SchemaVersion) || !recordRegion.MatchString(r.Region) {
			return &Error{"resources", "invalid"}
		}
		previous = r.ResourceID
	}
	s := d.snapshot(b).Message
	if m.BatchID != batchID(d.EnvironmentID, s.Revision, m.DestinationID, s.Resources) || m.DedupKey != "omr1:"+m.BatchID {
		return &Error{"message", "identity_mismatch"}
	}
	if len(m.Payload) > maxEnvelopeBytes || m.PayloadDigest != digest([]byte(m.Payload)) || !validSavedPayload(s, d.SchemaVersion) {
		return &Error{"message", "invalid_payload"}
	}
	return nil
}
