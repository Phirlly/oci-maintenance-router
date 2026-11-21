package noticeledger

import (
	"bytes"
	"encoding/json"
	"unicode/utf8"

	"oci-maintenance-router/internal/delivery"
)

// Decode accepts only the private encoder representation and trusted lookup
// identity. It validates consistency, not source authenticity or current policy.
func Decode(data []byte, environmentID, noticeID string) (Record, error) {
	if len(data) > recordLimit {
		return Record{}, &Error{"too_large"}
	}
	if !utf8.Valid(data) || !boundedJSON(data) {
		return Record{}, &Error{"invalid_json"}
	}
	w := wireRecord{recordData: &recordData{}}
	if json.Unmarshal(data, &w) != nil || w.recordData == nil {
		return Record{}, &Error{"invalid_schema"}
	}
	canonical, _ := json.Marshal(w)
	if !bytes.Equal(data, canonical) {
		return Record{}, &Error{"noncanonical"}
	}
	if w.EnvironmentID != environmentID || w.NoticeID != noticeID {
		return Record{}, &Error{"identity_mismatch"}
	}
	if !bytes.Equal(w.Delivery, []byte("null")) {
		var err error
		w.delivery, err = delivery.DecodeRecord(w.Delivery, environmentID, noticeID)
		if err != nil || w.delivery.SchemaVersion() != deliveryVersion(w.SchemaVersion) {
			return Record{}, &Error{"invalid_delivery"}
		}
	}
	if err := validate(w.recordData); err != nil {
		return Record{}, err
	}
	return Record{data: w.recordData}, nil
}

type wireRecord struct {
	*recordData
	Delivery json.RawMessage `json:"delivery"`
}

func (r Record) Encode() ([]byte, error) {
	if r.data == nil {
		return nil, &Error{"invalid_record"}
	}
	var deliveries json.RawMessage
	if len(r.data.delivery.Batches()) != 0 {
		deliveries, _ = r.data.delivery.Encode()
	}
	return json.Marshal(wireRecord{r.data, deliveries})
}
