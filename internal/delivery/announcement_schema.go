package delivery

import "regexp"

var recordInstanceID = regexp.MustCompile(`^ocid1\.instance\.oc1\.([a-z0-9][a-z0-9-]*)\.([A-Za-z0-9_-]*\.)?[A-Za-z0-9_-]+$`)

// MigrateAnnouncements proposes explicit v2-to-v3 migration without persistence.
// The enclosing notice must separately adopt this format before storing it.
func (r Record) MigrateAnnouncements() (Record, error) {
	if r.data == nil || r.data.SchemaVersion != 2 {
		return Record{}, &Error{"migration", "invalid"}
	}
	d := r.copy()
	d.SchemaVersion = 3
	// Only a same-width schema digit changes; v2 reserves the same metadata.
	return Record{data: &d}, nil
}

func validResourceID(id string, version int) bool {
	return recordDBID.MatchString(id) || (version == 3 && recordInstanceID.MatchString(id))
}
