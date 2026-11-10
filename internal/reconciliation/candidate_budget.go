package reconciliation

import "strings"

const (
	maxCandidateFacts = 10000
	maxCandidateBytes = 4 * 1024 * 1024
)

func (c *Candidates) admit(facts []candidateFact) error {
	if len(facts) > maxCandidateFacts-len(c.facts) {
		return c.fail(ItemLimit)
	}
	remaining := maxCandidateBytes - c.bytes
	for _, f := range facts {
		for _, text := range []string{f.revision.NoticeID, f.revision.Digest, f.chain} {
			if len(text) > remaining {
				return c.fail(ByteLimit)
			}
			remaining -= len(text)
		}
	}
	// Clone only after the entire admission fits. Keep neither source backing
	// buffers nor a partially admitted saved record on failure.
	for _, f := range facts {
		f.revision.NoticeID = strings.Clone(f.revision.NoticeID)
		f.revision.Digest = strings.Clone(f.revision.Digest)
		f.chain = strings.Clone(f.chain)
		c.facts = append(c.facts, f)
	}
	c.bytes = maxCandidateBytes - remaining
	return nil
}
