package reconciliation

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	"oci-maintenance-router/internal/noticeledger"
	"oci-maintenance-router/internal/routing"
)

func TestIssueReportRejectsForeignOrInvalidNoticeEvidenceWithoutMutation(t *testing.T) {
	f := issueReporterFixtureFor(t)
	r := f.processing.saved(t, noticeID(1))
	for _, kind := range []string{"foreign", "invalid-review"} {
		t.Run(kind, func(t *testing.T) {
			s := newReport(f.reporter.environment, "installation-1", reportTestChannel)
			config := fmt.Sprintf("%x", f.processing.p.controls.configHash)
			if kind == "foreign" {
				s.Environment = reportTestChannel
			} else {
				config = "invalid"
			}
			before, _ := encodeReport(s)
			err := mergeReportNotice(&s, r, noticeledger.PreparationIssues{}, true, config)
			after, _ := encodeReport(s)
			if err == nil || !bytes.Equal(before, after) {
				t.Fatal("invalid observation changed reporting state", err)
			}
		})
	}
}

func TestIssueReportKeepsUnknownOwnershipUntilPositiveResolution(t *testing.T) {
	n := processingNotice(t, 1, processingDB)
	f := processingFixtureFor(t, n)
	lookup := f.p.mysql["us-ashburn-1"]
	f.p.mysql["us-ashburn-1"] = func(context.Context, string) (routing.Metadata, error) {
		return routing.Metadata{ID: chain(processingDB), CompartmentID: chain("ocid1.compartment.oc1..example")}, nil
	}
	f.run(t)
	r := f.saved(t, n.Key().NoticeID)
	s := newReport(f.live.lease.manager.environmentID, "installation-1", reportTestChannel)
	config := fmt.Sprintf("%x", f.p.controls.configHash)
	if err := mergeReportNotice(&s, r, noticeledger.PreparationIssues{}, true, config); err != nil || len(s.Issues) != 1 || s.Issues[0].Kind != "RESOURCE" {
		t.Fatal("confirmed ownership problem was not retained", s.Issues, err)
	}
	first := s.Revision
	unknown, err := f.p.observer.Observe(r, n, true, nil, f.live.lease.manager.now())
	if err != nil {
		t.Fatal(err)
	}
	if err = mergeReportNotice(&s, unknown, noticeledger.PreparationIssues{}, true, config); err != nil || len(s.Issues) != 1 || s.Revision != first {
		t.Fatal("provisional lookup falsely resolved or changed the issue", err)
	}
	empty := newReport(s.Environment, s.Installation, s.Channel)
	if err = mergeReportNotice(&empty, unknown, noticeledger.PreparationIssues{}, true, config); err != nil || len(empty.Issues) != 0 {
		t.Fatal("provisional lookup generated a new failure", err)
	}
	f.p.mysql["us-ashburn-1"] = lookup
	f.run(t)
	if err = mergeReportNotice(&s, f.saved(t, n.Key().NoticeID), noticeledger.PreparationIssues{}, true, config); err != nil || len(s.Issues) != 0 || s.Revision <= first {
		t.Fatal("confirmed assignment did not resolve the known problem", err)
	}
	resolved := s.Revision
	if err = mergeReportNotice(&s, r, noticeledger.PreparationIssues{}, true, config); err != nil || len(s.Issues) != 1 || s.Revision <= resolved {
		t.Fatal("recurrence reused the earlier observation revision", err)
	}
}

func TestIssueReportKeepsPreparationRecheckAndUnrelatedNotices(t *testing.T) {
	n := processingNotice(t, 1, processingDB)
	f := processingFixtureFor(t, n)
	f.p.current = func(context.Context, string) (maintenanceCredential, error) { return nil, nil }
	f.run(t)
	r := f.saved(t, n.Key().NoticeID)
	p := savedPreparationIssues(t, f, n.Key().NoticeID)
	s := newReport(f.live.lease.manager.environmentID, "installation-1", reportTestChannel)
	config := fmt.Sprintf("%x", f.p.controls.configHash)
	if err := mergeReportNotice(&s, r, p, true, config); err != nil || len(s.Issues) != 1 || s.Issues[0].Kind != "PREPARATION" {
		t.Fatal("preparation failure omitted", s.Issues, err)
	}
	before := s.Revision
	if err := mergeReportNotice(&s, r, p, true, reportTestChannel); err != nil || len(s.Issues) != 1 || s.Revision != before {
		t.Fatal("changed context falsely resolved last known failure", err)
	}
	if err := mergeReportNotice(&s, r, noticeledger.PreparationIssues{}, false, config); err != nil || len(s.Issues) != 1 {
		t.Fatal("unreadable sidecar erased confirmed failure", err)
	}
	other := processingFixtureFor(t, processingNotice(t, 2, processingDB))
	other.run(t)
	if err := mergeReportNotice(&s, other.saved(t, noticeID(2)), noticeledger.PreparationIssues{}, true, config); err != nil || len(s.Issues) != 1 {
		t.Fatal("another notice resolved existing issue", err)
	}
}
