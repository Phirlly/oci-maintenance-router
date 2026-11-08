package notices

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"slices"
)

var announcementID = regexp.MustCompile("^ocid1\\.announcement\\.oc1\\.\\.([A-Za-z0-9_-]*\\.)?[A-Za-z0-9_-]+$")

// Normalize validates domain values and takes an owned snapshot. Unknown labels
// and missing resources are preserved for later classification, not authorized.
func Normalize(input Record) (Notice, error) {
	if err := validateInput(input); err != nil {
		return Notice{}, err
	}
	if !announcementID.MatchString(input.NoticeID) {
		return Notice{}, &Error{"noticeId", "invalid"}
	}
	if input.AnnouncementType == "" {
		return Notice{}, &Error{"announcementType", "required"}
	}
	if len(projection(input, true)) > maxNoticeBytes {
		return Notice{}, &Error{"notice", "too_large"}
	}
	r := cloneRecord(input)
	if err := normalizeTimes(&r); err != nil {
		return Notice{}, err
	}
	slices.Sort(r.Services)
	r.Services = slices.Compact(r.Services)
	resources, err := normalizeResources(r.Resources)
	if err != nil {
		return Notice{}, err
	}
	r.Resources = resources
	// Include context so every accepted snapshot can be normalized again.
	if len(projection(r, true)) > maxNoticeBytes {
		return Notice{}, &Error{"notice", "too_large"}
	}
	canonical := projection(r, false) // A subset of the bounded full snapshot.
	digest := sha256.Sum256(canonical)
	return Notice{record: r, key: RevisionKey{r.NoticeID, hex.EncodeToString(digest[:])}}, nil
}
