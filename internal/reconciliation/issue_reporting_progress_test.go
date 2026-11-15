package reconciliation

import (
	"context"
	"fmt"
	"slices"
	"testing"

	"oci-maintenance-router/internal/ocistorage"
)

func TestIssueReportingResumesObservedPageAndPassesUnreadableObjects(t *testing.T) {
	f := issueReporterFixtureFor(t)
	names := []string{}
	for i := 0; i < 25; i++ {
		names = append(names, fmt.Sprintf("notices/%064x.json", i))
	}
	pages := 0
	f.processing.live.objects.page = func(context.Context, string) (ocistorage.NoticePage, error) {
		pages++
		return ocistorage.NoticePage{Names: slices.Clone(names)}, nil
	}
	first, err := f.reporter.Refresh(leaseBudget(t), f.processing.live.lease, f.snapshot)
	if err != nil || first.Reviewed != 10 || f.saved(t).Cursor.Offset != 9 || pages != 1 {
		t.Fatal("bounded first batch did not advance through unreadable objects", first, err)
	}
	// Changing live membership cannot reset a page already observed and retained.
	names = []string{reportNoticeName(noticeID(1))}
	second, err := f.reporter.Refresh(leaseBudget(t), f.processing.live.lease, ProcessingSnapshot{})
	if err != nil || second.Reviewed != 10 || f.saved(t).Cursor.Offset != 19 || pages != 1 || f.sends != 1 {
		t.Fatal("saved page restarted, unreadable item starved progress, or cooldown lost", second, err)
	}
	if len(f.saved(t).Issues) != 20 {
		t.Fatal("confirmed preparation or unreadable-object problems were lost")
	}
}

func TestIssueReportingPriorityFIFOStillAdvancesRetainedSweep(t *testing.T) {
	f := issueReporterFixtureFor(t)
	snapshot := ProcessingSnapshot{}
	for i := 100; i < 120; i++ {
		snapshot.Issues = append(snapshot.Issues, ProcessingIssue{NoticeID: noticeID(i), Code: "not-authority"})
	}
	got, err := f.reporter.Refresh(leaseBudget(t), f.processing.live.lease, snapshot)
	if err != nil || got.Reviewed != 10 || len(f.saved(t).Priority) != 11 {
		t.Fatal("priority and retained sweep did not share the bound", got, err)
	}
	want := reportNoticeName(noticeID(109))
	if f.saved(t).Priority[0] != want {
		t.Fatal("priority hints did not retain FIFO progress")
	}
	got, err = f.reporter.Refresh(leaseBudget(t), f.processing.live.lease, snapshot)
	if err != nil || got.Reviewed != 10 || f.saved(t).Priority[0] != reportNoticeName(noticeID(118)) {
		t.Fatal("repeated hints starved earlier pending notices", got, err)
	}
}

func TestIssueReportingRejectsNonadvancingListWithoutClearingIssues(t *testing.T) {
	f := issueReporterFixtureFor(t)
	if _, err := f.reporter.Refresh(leaseBudget(t), f.processing.live.lease, f.snapshot); err != nil {
		t.Fatal(err)
	}
	s := f.saved(t)
	s.Cursor = reportCursor{Start: fmt.Sprintf("notices/%064x.json", 10), Names: []string{}}
	body, _ := encodeReport(s)
	f.processing.live.objects.seed(issueReportName, body)
	f.processing.live.objects.page = func(_ context.Context, start string) (ocistorage.NoticePage, error) {
		return ocistorage.NoticePage{Names: []string{}, NextStartWith: start}, nil
	}
	got, err := f.reporter.Refresh(leaseBudget(t), f.processing.live.lease, ProcessingSnapshot{})
	if err == nil || got.Issues != 1 || f.saved(t).Cursor.Start != s.Cursor.Start || f.sends != 1 {
		t.Fatal("invalid listing advanced or cleared known issues", got, err)
	}
}

func TestIssueReportingPriorityCapacityStillMakesProgress(t *testing.T) {
	f := issueReporterFixtureFor(t)
	snapshot := ProcessingSnapshot{}
	for i := 100; i < 1100; i++ {
		snapshot.Issues = append(snapshot.Issues, ProcessingIssue{NoticeID: noticeID(i)})
	}
	if _, err := f.reporter.Refresh(leaseBudget(t), f.processing.live.lease, snapshot); err != nil {
		t.Fatal(err)
	}
	// A changed full set overlaps the retained FIFO but adds one new hint.
	// Admission must not prevent draining previously confirmed work.
	snapshot.Issues[999].NoticeID = noticeID(1100)
	got, err := f.reporter.Refresh(leaseBudget(t), f.processing.live.lease, snapshot)
	if err != nil || got.Reviewed != 10 || !got.Known || got.Deferred != 1 || f.saved(t).Priority[0] != reportNoticeName(noticeID(118)) {
		t.Fatal("full hint queue blocked retained progress", got, err)
	}
}
