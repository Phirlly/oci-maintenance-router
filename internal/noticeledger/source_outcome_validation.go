package noticeledger

func validSourceOutcome(d *recordData) bool {
	if d.SchemaVersion == 1 {
		return d.SourceOutcome == nil
	}
	o := d.SourceOutcome
	if o == nil || !validTime(o.RecordedAt) || o.RecordedAt > d.UpdatedAt {
		return false
	}
	switch o.State {
	case SourceUnknown:
		return o.RevisionDigest == "" && o.HTTPStatus == 0
	case SourceAvailable:
		i := revisionIndex(d, o.RevisionDigest)
		return i >= 0 && o.HTTPStatus == 0 && d.Revisions[i].FirstSeenAt <= o.RecordedAt
	case SourceUnavailable:
		return o.RevisionDigest == "" && unavailableStatus(o.HTTPStatus) && d.HoldReason != ""
	}
	return false
}
