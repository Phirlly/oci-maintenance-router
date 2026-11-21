package delivery

import (
	"bytes"
	"encoding/json"
	"strings"

	"oci-maintenance-router/internal/notices"
)

// Decode only the shared context needed for validation. Comparing the existing
// closed projection rejects every other field/value change, including injected
// credentials or recipients. It never replaces saved retry bytes.
type savedContext struct {
	Payload struct {
		Source    string  `json:"source"`
		Timestamp *string `json:"timestamp"`
		Details   struct {
			FormatVersion    *int    `json:"formatVersion"`
			AnnouncementType *string `json:"announcementType"`
			Reference        *string `json:"reference"`
			Times            []struct {
				Slot  int     `json:"slot"`
				Type  *string `json:"type"`
				Title *string `json:"title"`
				Value *string `json:"value"`
			} `json:"times"`
		} `json:"custom_details"`
	} `json:"payload"`
}

func validSavedPayload(s Snapshot, version int) bool {
	var context savedContext
	if json.Unmarshal(s.PayloadWithoutRoutingKey, &context) != nil {
		return false
	}
	p := context.Payload
	name, ok := strings.CutPrefix(p.Source, "oci-maintenance-router/")
	if !ok || !recordName.MatchString(name) || len(p.Details.Times) != 2 {
		return false
	}
	r := notices.Record{NoticeID: s.Revision.NoticeID, AnnouncementType: "SCHEDULED_MAINTENANCE", Reference: p.Details.Reference, TimeUpdated: p.Timestamp}
	encode := payload
	if p.Details.FormatVersion != nil {
		if version != 3 || *p.Details.FormatVersion != 2 || p.Details.AnnouncementType == nil {
			return false
		}
		r.AnnouncementType = *p.Details.AnnouncementType
		if legacyAnnouncement(r.AnnouncementType, s.Resources) {
			return false
		}
		encode = announcementPayload
	} else if !legacyAnnouncement(r.AnnouncementType, s.Resources) {
		return false
	}
	for i, v := range p.Details.Times {
		if v.Slot != i+1 {
			return false
		}
		r.Times[i] = notices.SemanticTime{Type: v.Type, Title: v.Title, Value: v.Value}
	}
	// Source timestamp semantics include year 0000 and differ from attempt clocks.
	n, err := notices.Normalize(r)
	if err != nil {
		return false
	}
	return bytes.Equal(s.PayloadWithoutRoutingKey, encode(n.Snapshot(), s.Revision, name, s.DedupKey, s.Resources))
}
