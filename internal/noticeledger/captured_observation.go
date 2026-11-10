package noticeledger

import (
	"encoding/json"
	"strings"
	"time"

	"oci-maintenance-router/internal/notices"
)

// Observation owns a redacted, provisional source projection. It is reusable,
// but grants no relevance, freshness, durability or delivery authority.
type Observation struct {
	data *observationData
	size int
}

type observationData struct {
	EnvironmentID string   `json:"environmentId"`
	NoticeID      string   `json:"noticeId"`
	Revision      Revision `json:"revision"`
}

// Capture retains a potential candidate before complete relevance is known.
// No metadata lookup or complete-record capacity admission takes place here.
func (o *Observer) Capture(n notices.Notice) (Observation, error) {
	if o == nil || o.policy == nil || n.Key().NoticeID == "" {
		return Observation{}, &Error{"invalid_observation"}
	}
	d := observationData{
		EnvironmentID: strings.Clone(o.environmentID), NoticeID: strings.Clone(n.Key().NoticeID),
		Revision: projectRevision(n, o.policy.Resolve(n, true, nil)),
	}
	// These fixed fields contain only JSON-supported scalar/struct/slice types.
	b, _ := json.Marshal(d)
	return Observation{data: &d, size: len(b)}, nil
}

func (o Observation) Key() notices.RevisionKey {
	if o.data == nil {
		return notices.RevisionKey{}
	}
	return notices.RevisionKey{NoticeID: o.data.NoticeID, Digest: o.data.Revision.Digest}
}

// RetainedBytes measures the complete encoded projection, not peak heap usage.
func (o Observation) RetainedBytes() int { return o.size }

// Apply proposes one immediate record transition at the explicit merge time.
// It preserves receipts/history and admits the complete resulting record.
func (o Observation) Apply(previous Record, at time.Time) (Record, error) {
	if o.data == nil {
		return Record{}, &Error{"invalid_observation"}
	}
	stamp, err := previous.observationTime(o.data.EnvironmentID, o.data.NoticeID, at)
	if err != nil {
		return Record{}, err
	}
	return previous.mergeObservation(o.data.EnvironmentID, o.data.NoticeID, o.data.Revision, stamp, at)
}
