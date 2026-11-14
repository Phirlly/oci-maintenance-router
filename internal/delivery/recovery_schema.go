package delivery

// SchemaVersion describes the saved format without migrating it.
func (r Record) SchemaVersion() int {
	if r.data == nil {
		return 0
	}
	return r.data.SchemaVersion
}

// MigrateRecovery proposes explicit v1-to-v2 migration without persistence.
func (r Record) MigrateRecovery() (Record, error) {
	if r.data == nil || r.data.SchemaVersion != 1 {
		return Record{}, &Error{"migration", "invalid"}
	}
	d := r.copy()
	d.SchemaVersion = 2
	for i, b := range d.Batches {
		kind := lastAttemptKind(b)
		d.Batches[i].AttemptKind = &kind
	}
	if err := recordCapacity(&d); err != nil {
		return Record{}, err
	}
	return Record{data: &d}, nil
}

func lastAttemptKind(b savedBatch) AttemptKind {
	if b.AttemptKind != nil {
		return *b.AttemptKind
	}
	if b.AttemptCount > 0 {
		return AutomaticAttempt
	}
	return ""
}

func validAttemptKind(b savedBatch, version int) bool {
	if version == 1 {
		return b.AttemptKind == nil
	}
	if b.AttemptKind == nil {
		return false
	}
	if b.AttemptCount == 0 {
		return *b.AttemptKind == ""
	}
	return *b.AttemptKind == AutomaticAttempt || *b.AttemptKind == ManualAttempt
}
