package delivery

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	"oci-maintenance-router/internal/notices"
	"oci-maintenance-router/internal/routing"
)

func batchID(environmentID string, revision notices.RevisionKey, destination string, resources []routing.Target) string {
	ids := make([]string, 0, len(resources))
	for _, r := range resources {
		ids = append(ids, r.ResourceID)
	}
	// Closed Go-v1 tuple contains only integers, strings and string arrays.
	data, _ := json.Marshal([]any{1, environmentID, revision.NoticeID, revision.Digest, destination, ids})
	return digest(data)
}

func digest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
