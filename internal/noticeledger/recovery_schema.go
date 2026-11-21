package noticeledger

import "time"

func recoveryVersion(version int) bool { return version == 3 || version == 4 }

// MigrateRecovery proposes one explicit notice2/delivery1 to notice3/delivery2
// transition. It neither persists the proposal nor authorizes an action.
func (r Record) MigrateRecovery(at time.Time) (Record, error) {
	if r.data == nil || r.data.SchemaVersion != 2 {
		return Record{}, &Error{"invalid_migration"}
	}
	stamp, err := r.operationTime(at)
	if err != nil {
		return Record{}, err
	}
	d := r.data.copy()
	d.SchemaVersion, d.UpdatedAt = 3, stamp
	d.Recovery = &recoveryData{Actions: []RecoveryAction{}}
	if len(d.delivery.Batches()) != 0 {
		d.delivery, err = d.delivery.MigrateRecovery()
		if err != nil {
			return Record{}, err
		}
	}
	return r.finalize(d, at)
}
