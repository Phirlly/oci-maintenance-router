package reconciliation

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

// Hash the pinned compact JSON array without retaining an encoded copy of the
// whole component. Callers provide canonical sorted unique membership.
func componentFingerprint(environmentID string, members []CandidateRevision) string {
	h := sha256.New()
	_, _ = h.Write([]byte(`["oci-maintenance-router/candidate-component",1,`))
	environment, _ := json.Marshal(environmentID)
	_, _ = h.Write(environment)
	_, _ = h.Write([]byte(",["))
	for i, m := range members {
		if i != 0 {
			_, _ = h.Write([]byte(","))
		}
		tuple, _ := json.Marshal([3]string{m.Revision.NoticeID, m.Revision.Digest, m.ChainID})
		_, _ = h.Write(tuple)
	}
	_, _ = h.Write([]byte("]]"))
	return hex.EncodeToString(h.Sum(nil))
}
