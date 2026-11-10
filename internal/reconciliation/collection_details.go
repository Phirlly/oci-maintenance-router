package reconciliation

import (
	"oci-maintenance-router/internal/noticeledger"
	"strings"
)

const observationByteLimit = 4 << 20

func (e *enumeration) currentDetails(source AnnouncementSource, ids []string, facts *Candidates, observer *noticeledger.Observer) ([]noticeledger.Observation, error) {
	var observations []noticeledger.Observation
	remaining := observationByteLimit
	for _, id := range ids {
		if err := e.before(Announcements); err != nil {
			return nil, collectionFailure(e.ctx, Announcements, err)
		}
		n, err := source.Get(e.ctx, id)
		if err != nil || e.ctx.Err() != nil {
			return nil, (*CollectionError)(readFailure(e.ctx, Announcements, err))
		}
		if n.Key().NoticeID == "" || n.Key().NoticeID != id {
			return nil, (*CollectionError)(failure(e.ctx, Announcements, InvalidNotice))
		}
		if err := facts.Observe(n); err != nil {
			return nil, collectionFailure(e.ctx, Announcements, err)
		}
		observation, err := observer.Capture(n)
		if err != nil {
			return nil, (*CollectionError)(failure(e.ctx, Announcements, InvalidNotice))
		}
		if e.ctx.Err() != nil {
			return nil, (*CollectionError)(failure(e.ctx, Announcements, Canceled))
		}
		if observation.RetainedBytes() > remaining {
			return nil, (*CollectionError)(failure(e.ctx, Announcements, ByteLimit))
		}
		remaining -= observation.RetainedBytes()
		observations = append(observations, observation)
	}
	return observations, nil
}

func (e *enumeration) savedDetails(source SavedNoticeSource, names []string, facts *Candidates) ([]SavedNoticeRef, error) {
	var result []SavedNoticeRef
	for _, name := range names {
		if err := e.before(SavedNotices); err != nil {
			return nil, collectionFailure(e.ctx, SavedNotices, err)
		}
		r, err := source.LoadRecord(e.ctx, name)
		if err != nil || e.ctx.Err() != nil {
			return nil, (*CollectionError)(readFailure(e.ctx, SavedNotices, err))
		}
		if err := facts.Restore(r); err != nil {
			return nil, collectionFailure(e.ctx, SavedNotices, err)
		}
		if e.ctx.Err() != nil {
			return nil, (*CollectionError)(failure(e.ctx, SavedNotices, Canceled))
		}
		_, id := r.Identity()
		// Names are already owned by enumeration. Do not keep a record's backing
		// buffer alive through its ID; no full record or CAS handle is retained.
		result = append(result, SavedNoticeRef{ObjectName: name, NoticeID: strings.Clone(id)})
	}
	return result, nil
}
