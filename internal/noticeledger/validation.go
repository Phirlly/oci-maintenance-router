package noticeledger

import (
	"bytes"
	"encoding/json"
	"regexp"
	"time"

	"oci-maintenance-router/internal/delivery"
	"oci-maintenance-router/internal/notices"
	"oci-maintenance-router/internal/routing"
)

var digestPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)
var noticePattern = regexp.MustCompile(`^ocid1\.announcement\.oc1\.\.([A-Za-z0-9_-]*\.)?[A-Za-z0-9_-]+$`)
var dbPattern = regexp.MustCompile(`^ocid1\.mysqldbsystem\.oc1\.([a-z0-9][a-z0-9-]*)\.([A-Za-z0-9_-]*\.)?[A-Za-z0-9_-]+$`)
var regionPattern = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
var destinationPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,31}$`)

func validTime(s string) bool {
	t, e := time.Parse(timeFormat, s)
	return e == nil && !t.IsZero() && t.Year() >= 1 && t.Year() <= 9999 && t.Format(timeFormat) == s
}
func validSource(s Source, noticeID string) bool {
	r := notices.Record{NoticeID: noticeID, AnnouncementType: "STATE_CONTEXT", ChainID: s.ChainID, Reference: s.Reference, TimeCreated: s.TimeCreated, TimeUpdated: s.TimeUpdated}
	for i, t := range s.Times {
		r.Times[i] = notices.SemanticTime{Type: t.Type, Title: t.Title, Value: t.Value}
	}
	n, e := notices.Normalize(r)
	if e != nil {
		return false
	}
	want, _ := json.Marshal(sourceContext(n.Snapshot()))
	got, _ := json.Marshal(s)
	return bytes.Equal(want, got)
}
func validate(d *recordData) error {
	if (d.SchemaVersion != 1 && d.SchemaVersion != 2) || !digestPattern.MatchString(d.EnvironmentID) || !noticePattern.MatchString(d.NoticeID) || !validTime(d.UpdatedAt) || len(d.Revisions) == 0 || (d.HoldReason != "" && !holdReason(d.HoldReason)) || (len(d.Revisions) > 1 && d.HoldReason == "") {
		return &Error{"invalid_record"}
	}
	previous := ""
	for _, v := range d.Revisions {
		if !digestPattern.MatchString(v.Digest) || v.Digest <= previous || !digestPattern.MatchString(v.RoutingGeneration) || !validTime(v.FirstSeenAt) || !validTime(v.LastSeenAt) || v.FirstSeenAt > v.LastSeenAt || v.LastSeenAt > d.UpdatedAt || !validSource(v.Source, d.NoticeID) {
			return &Error{"invalid_revision"}
		}
		previous = v.Digest
		if len(v.Resources) > 256 || !validAssessment(v) {
			return &Error{"invalid_assessment"}
		}
		lastID := ""
		for i, row := range v.Resources {
			if row.Index != i || !validResource(row, v, d.HoldReason) {
				return &Error{"invalid_resource"}
			}
			if row.ResourceID != "" {
				if row.ResourceID <= lastID {
					return &Error{"duplicate_resource"}
				}
				lastID = row.ResourceID
			}
			if row.BatchID != "" {
				b, ok := d.delivery.Batch(row.BatchID)
				if !ok || b.Message.Revision.Digest != v.Digest {
					return &Error{"orphan_assignment"}
				}
				found := false
				for _, m := range b.Message.Resources {
					found = found || m.ResourceID == row.ResourceID
				}
				if !found {
					return &Error{"orphan_assignment"}
				}
			}
		}
	}
	if !validSourceOutcome(d) {
		return &Error{"invalid_source_outcome"}
	}
	for _, b := range d.delivery.Batches() {
		i := revisionIndex(d, b.Message.Revision.Digest)
		if i < 0 || (!b.UpdatedAt.IsZero() && b.UpdatedAt.Format(timeFormat) > d.UpdatedAt) || (!b.LastAttemptAt.IsZero() && b.LastAttemptAt.Format(timeFormat) < d.Revisions[i].FirstSeenAt) {
			return &Error{"invalid_delivery_time"}
		}
		if effectiveHold(d, b) != "" && (b.State == delivery.Prepared || b.State == delivery.RetryWait) {
			return &Error{"unapplied_hold"}
		}
	}
	expected := d.copy()
	if e := synchronize(&expected); e != nil {
		return e
	}
	want, _ := json.Marshal(expected.Revisions)
	got, _ := json.Marshal(d.Revisions)
	if !bytes.Equal(want, got) {
		return &Error{"inconsistent_dispositions"}
	}
	return capacity(d)
}
func validAssessment(v Revision) bool {
	if v.Assessment == routing.SupportedShape {
		return v.AssessmentReason == "" && len(v.Resources) > 0
	}
	if v.Assessment != routing.ReviewRequired {
		return false
	}
	switch v.AssessmentReason {
	case routing.ReasonService, routing.ReasonType, routing.ReasonPlatform, routing.ReasonNoResources:
		return true
	}
	return false
}
