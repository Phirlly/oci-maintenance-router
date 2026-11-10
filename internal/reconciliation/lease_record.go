package reconciliation

import (
	"bytes"
	"encoding/json"
	"regexp"
	"time"
)

const workerLeaseName = "control/lease.json"
const leaseTimeFormat = "2006-01-02T15:04:05.000Z"
const leaseDuration = 600 * time.Second
const leaseWorkDuration = 240 * time.Second

var leaseDigest = regexp.MustCompile("^[a-f0-9]{64}$")

type leaseRecord struct {
	SchemaVersion int    `json:"schemaVersion"`
	EnvironmentID string `json:"environmentId"`
	OwnerToken    string `json:"ownerToken"`
	AcquiredAt    string `json:"acquiredAt"`
	ExpiresAt     string `json:"expiresAt"`
}

func (r leaseRecord) bytes() []byte  { body, _ := json.Marshal(r); return body }
func leaseWallTime(t time.Time) bool { return !t.IsZero() && t.Year() >= 1 && t.Year() <= 9999 }
func leaseTimestamp(value string) (time.Time, bool) {
	t, e := time.Parse(leaseTimeFormat, value)
	return t, e == nil && leaseWallTime(t) && t.Format(leaseTimeFormat) == value
}
func decodeLease(body []byte, environment string) (leaseRecord, error) {
	var r leaseRecord
	if len(body) > 4096 || json.Unmarshal(body, &r) != nil {
		return r, &LeaseError{Code: "invalid_record"}
	}
	a, okA := leaseTimestamp(r.AcquiredAt)
	e, okE := leaseTimestamp(r.ExpiresAt)
	if r.SchemaVersion != 1 || r.EnvironmentID != environment || !leaseDigest.MatchString(r.OwnerToken) || !okA || !okE || (e.Sub(a) != leaseDuration && !e.Equal(a)) || !bytes.Equal(body, r.bytes()) {
		return leaseRecord{}, &LeaseError{Code: "invalid_record"}
	}
	return r, nil
}
func (r leaseRecord) times() (time.Time, time.Time) {
	a, _ := leaseTimestamp(r.AcquiredAt)
	e, _ := leaseTimestamp(r.ExpiresAt)
	return a, e
}
func (r leaseRecord) clock(now time.Time) error {
	a, e := r.times()
	if !leaseWallTime(now) || now.Before(a) {
		return &LeaseError{Code: "clock_invalid"}
	}
	if !now.Before(e) {
		return &LeaseError{Code: "ownership_lost"}
	}
	return nil
}
