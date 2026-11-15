package deployment

import "oci-maintenance-router/internal/noticeledger"

func closureItems(record noticeledger.Record, generation string, request command) ([]any, error) {
	s := record.Snapshot()
	blocker := ""
	if request.kind != "" {
		var err error
		blocker, err = record.ClosureBlocker(noticeledger.ActionKind(request.kind), request.targetID)
		if err != nil || blocker == "target_missing" {
			return nil, operationFailure("closure_target_missing")
		}
	}
	revision := ""
	if request.kind == string(noticeledger.CloseRevision) {
		revision = request.targetID
	}
	if request.kind == string(noticeledger.CloseBatch) {
		for _, b := range s.Batches {
			if b.Message.BatchID == request.targetID {
				revision = b.Message.Revision.Digest
			}
		}
	}
	items := []any{closureNoticeItem{"NOTICE", s.NoticeID, string(s.HoldReason), string(s.SourceOutcome.State), s.SourceOutcome.RevisionDigest, s.SourceOutcome.HTTPStatus, s.SourceOutcome.RecordedAt}}
	revisions := s.Revisions
	if request.kind != "" {
		revisions = nil
		for _, v := range s.Revisions {
			if v.Digest == revision {
				revisions = append(revisions, v)
			}
		}
	}
	items = append(items, closureRevisionItems(record, revisions, request)...)
	for _, b := range s.Batches {
		if request.kind == string(noticeledger.CloseBatch) && b.Message.BatchID != request.targetID {
			continue
		}
		if request.kind == string(noticeledger.CloseRevision) && b.Message.Revision.Digest != revision {
			continue
		}
		items = append(items, closureBatch(record, b))
	}
	items = append(items, closureHistory(s.Recovery)...)
	if request.kind != "" && blocker == "" {
		code, err := makeClosureCode(record, generation, request)
		if err != nil {
			return nil, err
		}
		items = append(items, code)
	}
	return items, nil
}
