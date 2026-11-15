package reconciliation

import (
	"fmt"
	"slices"

	"oci-maintenance-router/internal/ocistorage"
)

func (r *reportingRun) refresh(snapshot ProcessingSnapshot) error {
	state := cloneReport(r.saved.state)
	seen := map[string]bool{}
	for _, name := range state.Priority {
		seen[name] = true
	}
	for _, issue := range snapshot.Issues {
		if len(issue.NoticeID) > 4096 || !reportNoticePattern.MatchString(issue.NoticeID) {
			return &ReportingError{"invalid_hint"}
		}
		name := reportNoticeName(issue.NoticeID)
		if !seen[name] {
			seen[name] = true
			if len(state.Priority) == 1000 {
				r.result.Deferred++
				continue
			}
			state.Priority = append(state.Priority, name)
		}
	}
	if err := r.save(state, false); err != nil {
		return err
	}
	store, _ := ocistorage.NewNoticeStore(r.owner.objects, r.owner.environment)
	sweepDone := false
	for count := 0; count < 10; count++ {
		if err := r.guard(); err != nil {
			return err
		}
		state = cloneReport(r.saved.state)
		// First give fresh processing problems a turn, then guarantee a retained
		// sweep turn before continuing priority work. Neither list grants authority.
		priority := len(state.Priority) > 0 && (count != 1 || sweepDone)
		if !priority && sweepDone {
			break
		}
		name := ""
		if priority {
			name = state.Priority[0]
			state.Priority = slices.Clone(state.Priority[1:])
		} else {
			var err error
			name, sweepDone, err = nextReportNotice(r.ctx, store, &state.Cursor)
			if err != nil {
				return err
			}
		}
		if name != "" {
			if err := r.inspect(store, &state, name); err != nil {
				return err
			}
		}
		if err := r.save(state, false); err != nil {
			return err
		}
		if name != "" {
			r.result.Reviewed++
		}
	}
	return nil
}

func (r *reportingRun) inspect(store *ocistorage.NoticeStore, state *reportState, name string) error {
	record, err := store.LoadRecord(r.ctx, name)
	if r.ctx.Err() != nil {
		return &ReportingError{"deadline_exceeded"}
	}
	if err != nil {
		return retainReportStorageIssue(state, name, "notice_unreadable")
	}
	if err = r.guard(); err != nil {
		return err
	}
	preparations, _ := NewPreparationIssueStore(r.owner.objects, r.owner.environment)
	saved, failure := preparations.Load(r.ctx, record)
	if r.ctx.Err() != nil {
		return &ReportingError{"deadline_exceeded"}
	}
	if err = mergeReportNotice(state, record, saved.Record(), failure == nil, fmt.Sprintf("%x", r.owner.controls.configHash)); err != nil {
		return err
	}
	if failure != nil {
		return retainReportStorageIssue(state, name, "preparation_unreadable")
	}
	return nil
}

func retainReportStorageIssue(state *reportState, name, code string) error {
	issue := reportIssue{ObjectName: name, Kind: "STORE", Code: code, ResourceIndex: -1}
	for _, old := range state.Issues {
		if old == issue {
			return nil
		}
	}
	return replaceReportIssues(state, append(slices.Clone(state.Issues), issue))
}
