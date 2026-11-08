package delivery

import (
	"bytes"
	"encoding/json"
	"regexp"
	"unicode/utf8"
)

const maxRecordBytes = 1 << 20

var (
	recordDigest = regexp.MustCompile(`^[a-f0-9]{64}$`)
	recordOwner  = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)
	recordDBID   = regexp.MustCompile(`^ocid1\.mysqldbsystem\.oc1\.([a-z0-9][a-z0-9-]*)\.([A-Za-z0-9_-]*\.)?[A-Za-z0-9_-]+$`)
)

// Encode returns owned canonical v1 bytes. Writing them is the caller's separate
// conditional-storage operation; this method cannot establish durability.
func (r Record) Encode() ([]byte, error) {
	if r.data == nil {
		return nil, &Error{"record", "invalid"}
	}
	// Only constructors, validated decoding and closed transitions create Record.
	data, _ := json.Marshal(r.data)
	return data, nil
}

// DecodeRecord accepts only the exact private encoder format. expected identities
// come from the trusted storage lookup, not from these bytes or current settings.
func DecodeRecord(data []byte, environmentID, noticeID string) (Record, error) {
	if len(data) > maxRecordBytes {
		return Record{}, &Error{"record", "too_large"}
	}
	if !utf8.Valid(data) || !boundedRecordJSON(data) {
		return Record{}, &Error{"record", "invalid_json"}
	}
	var d recordData
	if json.Unmarshal(data, &d) != nil {
		return Record{}, &Error{"record", "invalid_schema"}
	}
	canonical, _ := json.Marshal(d)
	if !bytes.Equal(canonical, data) {
		return Record{}, &Error{"record", "noncanonical"}
	}
	if d.EnvironmentID != environmentID || d.NoticeID != noticeID {
		return Record{}, &Error{"record", "identity_mismatch"}
	}
	if err := validateRecord(&d); err != nil {
		return Record{}, err
	}
	return Record{data: &d}, nil
}
