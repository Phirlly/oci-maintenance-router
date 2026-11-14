package reconciliation

import (
	"bytes"
	"encoding/json"
	"unicode/utf8"
)

const operatorChallengeLimit = 16 << 10

func (r OperatorChallenge) Encode() ([]byte, error) {
	if r.data == nil {
		return nil, &OperatorChallengeError{Code: "invalid_record"}
	}
	// Closed construction/transitions and field bounds keep every state below
	// operatorChallengeLimit, including maximum escaped publication identifiers.
	body, _ := json.Marshal(r.data)
	return body, nil
}

// DecodeOperatorChallenge verifies private canonical state and trusted lookup
// identity, not persistence, current channel association or publication authority.
func DecodeOperatorChallenge(body []byte, environment, installation, request string) (OperatorChallenge, error) {
	if len(body) > operatorChallengeLimit {
		return OperatorChallenge{}, &OperatorChallengeError{Code: "too_large"}
	}
	d := &operatorChallengeData{}
	if !utf8.Valid(body) || json.Unmarshal(body, d) != nil {
		return OperatorChallenge{}, &OperatorChallengeError{Code: "invalid_json"}
	}
	if !validOperatorChallengeData(d) {
		return OperatorChallenge{}, &OperatorChallengeError{Code: "invalid_record"}
	}
	if d.Intent.EnvironmentID != environment || d.Intent.InstallationID != installation || d.Intent.RequestID != request {
		return OperatorChallenge{}, &OperatorChallengeError{Code: "identity_mismatch"}
	}
	canonical, _ := json.Marshal(d)
	if !bytes.Equal(body, canonical) {
		return OperatorChallenge{}, &OperatorChallengeError{Code: "noncanonical"}
	}
	return OperatorChallenge{data: d}, nil
}
