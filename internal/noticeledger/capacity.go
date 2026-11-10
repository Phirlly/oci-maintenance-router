package noticeledger

import (
	"encoding/json"
	"strings"

	"oci-maintenance-router/internal/delivery"
)

const recordLimit = 1 << 20

func capacity(d *recordData) error {
	// Reserve outer transitions as well as delivery transitions. All controlled
	// replacement fields are ASCII; source text keeps its actual encoded size.
	worst := d.copy()
	worst.HoldReason = delivery.ReasonConfigurationChanged
	if worst.SchemaVersion == 2 {
		// AVAILABLE's 64-byte digest exceeds every other permitted outcome,
		// including UNAVAILABLE's longer state and three-digit status.
		worst.SourceOutcome = &SourceOutcome{State: SourceAvailable, RecordedAt: d.UpdatedAt, RevisionDigest: strings.Repeat("f", 64)}
	}
	for i := range worst.Revisions {
		for j := range worst.Revisions[i].Resources {
			row := &worst.Revisions[i].Resources[j]
			row.Disposition = "REVIEW_REQUIRED"
			row.Reason = strings.Repeat("x", 32)
			row.BatchID = strings.Repeat("x", 64)
		}
	}
	data, _ := json.Marshal(wireRecord{recordData: &worst})
	size := len(data)
	if len(d.delivery.Batches()) != 0 {
		reserved, err := d.delivery.ReservedBytes()
		if err != nil {
			return err
		}
		size += reserved - len("null")
	}
	if size > recordLimit {
		return &Error{"too_large"}
	}
	return nil
}
