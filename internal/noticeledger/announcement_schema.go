package noticeledger

import (
	"regexp"
	"time"
)

var instancePattern = regexp.MustCompile(`^ocid1\.instance\.oc1\.([a-z0-9][a-z0-9-]*)\.([A-Za-z0-9_-]*\.)?[A-Za-z0-9_-]+$`)

// MigrateAnnouncements proposes explicit notice3/delivery2 to notice4/delivery3
// adoption. Persist this immediate-parent proposal before any further transition.
func (r Record) MigrateAnnouncements(at time.Time) (Record, error) {
	if r.data == nil || r.data.SchemaVersion != 3 {
		return Record{}, &Error{"invalid_migration"}
	}
	stamp, err := r.operationTime(at)
	if err != nil {
		return Record{}, err
	}
	d := r.data.copy()
	d.SchemaVersion, d.UpdatedAt = 4, stamp
	if len(d.delivery.Batches()) != 0 {
		d.delivery, err = d.delivery.MigrateAnnouncements()
		if err != nil {
			return Record{}, err
		}
	}
	return r.finalize(d, at)
}

func validResourceID(id string, version int) bool {
	return dbPattern.MatchString(id) || (version == 4 && instancePattern.MatchString(id))
}
