package noticeledger

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"unicode/utf8"
)

func preparationNoticeDigest(id string) string {
	hash := sha256.Sum256([]byte(id))
	return hex.EncodeToString(hash[:])
}

// Publication timestamps are outside revision identity but can affect payload
// size. Bind that closed source context together with the exact configuration.
func preparationContextDigest(config string, source Source) string {
	body, _ := json.Marshal([]any{"oci-maintenance-router/preparation-context", 1, config, source})
	hash := sha256.Sum256(body)
	return hex.EncodeToString(hash[:])
}

func (p PreparationIssues) Encode() ([]byte, error) {
	if p.data == nil {
		return nil, &Error{"invalid_preparation_issues"}
	}
	return json.Marshal(p.data)
}

// DecodePreparationIssues requires exact canonical bytes and the genuine notice
// whose retained indexed revisions give the observations their meaning.
func DecodePreparationIssues(body []byte, r Record) (PreparationIssues, error) {
	if len(body) > recordLimit {
		return PreparationIssues{}, &Error{"too_large"}
	}
	if !utf8.Valid(body) || !boundedJSON(body) {
		return PreparationIssues{}, &Error{"invalid_json"}
	}
	var d PreparationIssueSnapshot
	if json.Unmarshal(body, &d) != nil {
		return PreparationIssues{}, &Error{"invalid_schema"}
	}
	canonical, _ := json.Marshal(d)
	if !bytes.Equal(body, canonical) {
		return PreparationIssues{}, &Error{"noncanonical"}
	}
	p := PreparationIssues{data: &d}
	if err := p.validate(r); err != nil {
		return PreparationIssues{}, err
	}
	return p, nil
}

// Matches validates retained references; it does not imply current source truth.
func (p PreparationIssues) Matches(r Record) bool { return p.validate(r) == nil }

func (p PreparationIssues) validate(r Record) error {
	d := p.data
	if d == nil || r.data == nil || d.SchemaVersion != 1 || d.EnvironmentID != r.data.EnvironmentID || d.NoticeDigest != preparationNoticeDigest(r.data.NoticeID) || len(d.Observations) == 0 {
		return &Error{"invalid_preparation_issues"}
	}
	if len(d.Observations) > 2048 {
		return &Error{"too_large"}
	}
	for j, v := range d.Observations {
		i := revisionIndex(r.data, v.RevisionDigest)
		if i < 0 || v.ResourceIndex < 0 || v.ResourceIndex >= len(r.data.Revisions[i].Resources) || (j > 0 && comparePreparation(d.Observations[j-1], v) >= 0) || !preparationCode(v.Code) || !validTime(v.FirstSeenAt) || !validTime(v.LastSeenAt) || v.FirstSeenAt > v.LastSeenAt || v.FirstSeenAt < r.data.Revisions[i].FirstSeenAt || !digestPattern.MatchString(v.ContextDigest) || !digestPattern.MatchString(v.RoutingGeneration) || !destinationPattern.MatchString(v.DestinationID) {
			return &Error{"invalid_preparation_observation"}
		}
	}
	body, _ := p.Encode()
	if len(body) > recordLimit {
		return &Error{"too_large"}
	}
	return nil
}
