package reconciliation

import (
	"context"

	"oci-maintenance-router/internal/noticeledger"
	"oci-maintenance-router/internal/notices"
)

const observationByteLimit = 4 << 20

// One shared allowance covers listed and saved-only successful source reads.
type sourceObservations struct {
	facts     *Candidates
	observer  *noticeledger.Observer
	values    []noticeledger.Observation
	remaining int
}

func (s *sourceObservations) add(ctx context.Context, n notices.Notice) error {
	if err := s.facts.Observe(n); err != nil {
		return collectionFailure(ctx, Announcements, err)
	}
	observation, err := s.observer.Capture(n)
	if err != nil {
		return (*CollectionError)(failure(ctx, Announcements, InvalidNotice))
	}
	if ctx.Err() != nil {
		return (*CollectionError)(failure(ctx, Announcements, Canceled))
	}
	if observation.RetainedBytes() > s.remaining {
		return (*CollectionError)(failure(ctx, Announcements, ByteLimit))
	}
	s.remaining -= observation.RetainedBytes()
	s.values = append(s.values, observation)
	return nil
}
