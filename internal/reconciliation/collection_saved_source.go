package reconciliation

import "oci-maintenance-router/internal/ociannouncements"

func (e *enumeration) savedSources(source AnnouncementSource, listed []string, saved []SavedNoticeRef, observations *sourceObservations) ([]UnavailableNotice, error) {
	seen := make(map[string]bool, len(listed))
	for _, id := range listed {
		seen[id] = true
	}
	unavailable := []UnavailableNotice{}
	for _, ref := range saved {
		if seen[ref.NoticeID] {
			continue
		}
		seen[ref.NoticeID] = true
		if err := e.before(Announcements); err != nil {
			return nil, collectionFailure(e.ctx, Announcements, err)
		}
		n, err := source.Get(e.ctx, ref.NoticeID)
		if e.ctx.Err() != nil {
			return nil, (*CollectionError)(readFailure(e.ctx, Announcements, err))
		}
		if err != nil {
			status, ok := unavailableStatus(err)
			if !ok || n.Key().NoticeID != "" {
				return nil, (*CollectionError)(readFailure(e.ctx, Announcements, err))
			}
			unavailable = append(unavailable, UnavailableNotice{NoticeID: ref.NoticeID, HTTPStatus: status})
			continue
		}
		if n.Key().NoticeID == "" || n.Key().NoticeID != ref.NoticeID {
			return nil, (*CollectionError)(failure(e.ctx, Announcements, InvalidNotice))
		}
		if err := observations.add(e.ctx, n); err != nil {
			return nil, err
		}
	}
	return unavailable, nil
}

func unavailableStatus(err error) (int, bool) {
	// Only the adapter's direct controlled service failure is classified. Wrapped
	// or joined errors can hide fatal causes; Transport also includes header limits.
	detail, ok := err.(*ociannouncements.Error)
	if !ok || detail == nil || detail.Code != ociannouncements.Service {
		return 0, false
	}
	switch detail.Status {
	case 401, 403, 404, 408, 429, 500, 502, 503, 504:
		return detail.Status, true
	}
	return 0, false
}
