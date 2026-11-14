package delivery

import (
	"encoding/json"
	"math"
	"strings"
)

// ReservedBytes includes the largest version-specific transition metadata.
// An enclosing notice must add its own source/disposition overhead to this bound.
func (r Record) ReservedBytes() (int, error) {
	if r.data == nil {
		return 0, &Error{"record", "invalid"}
	}
	return reservedRecordBytes(r.data)
}

func recordCapacity(d *recordData) error {
	_, err := reservedRecordBytes(d)
	return err
}

func reservedRecordBytes(d *recordData) (int, error) {
	// Encode a closed upper-bound state, not a transition or valid receipt.
	// All optional times are present; ASCII fields never need JSON escaping.
	header := *d
	header.Batches = []savedBatch{}
	data, _ := json.Marshal(header)
	size := len(data)
	stamp := "9999-12-31T23:59:59.999Z"
	for i, b := range d.Batches {
		if d.SchemaVersion == 2 {
			kind := AutomaticAttempt
			b.AttemptKind = &kind
		}
		b.State, b.Reason = ReviewRequired, ReasonConfigurationChanged
		b.AttemptCount, b.OwnerToken = math.MaxUint64, strings.Repeat("x", 128)
		b.FirstAttemptAt, b.LastAttemptAt, b.AttemptExpiresAt = &stamp, &stamp, &stamp
		b.UpdatedAt, b.NextAttemptAt, b.AcceptedAt = &stamp, &stamp, &stamp
		b.Ambiguous = false // false is longer than true.
		// Count one batch at a time and stop as soon as it cannot fit. Do not
		// allocate one large serialization of every rejected preparation.
		data, _ = json.Marshal(b)
		size += len(data)
		if i > 0 {
			size++
		} // Comma between array members.
		if size > maxRecordBytes {
			return 0, &Error{"record", "too_large"}
		}
	}
	return size, nil
}
