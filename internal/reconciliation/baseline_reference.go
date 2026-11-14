package reconciliation

import (
	"crypto/sha256"
	"fmt"
)

// baselineReference binds an ID to the exact canonical immutable contents.
type baselineReference struct {
	ID     string `json:"baselineId"`
	Digest string `json:"contentSha256"`
}

func referenceForBaseline(b Baseline) baselineReference {
	body, _ := b.Encode()
	return baselineReference{b.ID(), fmt.Sprintf("%x", sha256.Sum256(body))}
}
