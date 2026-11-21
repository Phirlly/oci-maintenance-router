package reconciliation

import (
	"bytes"
	"errors"

	"oci-maintenance-router/internal/delivery"
	"oci-maintenance-router/internal/noticeledger"
	"oci-maintenance-router/internal/ocistorage"
)

func (r *discoveryRun) persist(work *discoveryWork) error {
	var saved ocistorage.StoredNotice
	if work.saved.NoticeID != "" {
		if err := r.guard(); err != nil {
			return err
		}
		var err error
		saved, err = r.store.LoadObject(r.ctx, work.saved.ObjectName)
		if err != nil {
			return discoveryFailureFor(r.ctx, "record_read", err)
		}
		if savedMembership(saved.Record()) != work.saved.membership {
			return &DiscoveryError{Code: "membership_changed"}
		}
		saved, err = r.migrate(saved)
		if err != nil {
			return err
		}
	}
	for _, observation := range work.observations {
		next, err := observation.Apply(saved.Record(), r.owner.now())
		if err != nil {
			return &DiscoveryError{Code: "transition_failed"}
		}
		saved, err = r.save(saved, next)
		if err != nil {
			return err
		}
	}
	if work.unavailable != 0 {
		next, err := saved.Record().RecordSourceUnavailable(work.unavailable, r.owner.now())
		if err != nil {
			return &DiscoveryError{Code: "transition_failed"}
		}
		saved, err = r.save(saved, next)
		if err != nil {
			return err
		}
	}
	if (work.hold && saved.Record().Snapshot().HoldReason == "") || recoveryComponentChanged(saved.Record(), work.component) {
		next, err := saved.Record().Hold(delivery.ReasonSourceChanged, r.owner.now())
		if err != nil {
			return &DiscoveryError{Code: "transition_failed"}
		}
		if _, err := r.save(saved, next); err != nil {
			return err
		}
	}
	return nil
}

func (r *discoveryRun) migrate(saved ocistorage.StoredNotice) (ocistorage.StoredNotice, error) {
	for {
		version := saved.Record().Snapshot().SchemaVersion
		if !r.owner.announcementFormat && version == 4 {
			return saved, &DiscoveryError{Code: "unsupported_notice_schema"}
		}
		if version == 4 || (!r.owner.announcementFormat && version >= 2) {
			return saved, nil
		}
		var next noticeledger.Record
		var err error
		switch version {
		case 1:
			next, err = saved.Record().MigrateSourceOutcome(r.owner.now())
		case 2:
			next, err = saved.Record().MigrateRecovery(r.owner.now())
		case 3:
			next, err = saved.Record().MigrateAnnouncements(r.owner.now())
		default:
			return saved, &DiscoveryError{Code: "unsupported_notice_schema"}
		}
		if err != nil {
			return saved, &DiscoveryError{Code: "transition_failed"}
		}
		saved, err = r.save(saved, next)
		if err != nil {
			return saved, err
		}
	}
}

func recoveryComponentChanged(record noticeledger.Record, component string) bool {
	if history := record.Snapshot().Recovery; history != nil {
		for _, a := range history.Actions {
			if a.Status == noticeledger.RecoveryApproved && a.InvalidatedAt == nil && a.Request.ComponentFingerprint != component {
				return true
			}
		}
	}
	return false
}

func (r *discoveryRun) save(previous ocistorage.StoredNotice, next noticeledger.Record) (ocistorage.StoredNotice, error) {
	if err := r.guard(); err != nil {
		return ocistorage.StoredNotice{}, err
	}
	var saved ocistorage.StoredNotice
	var err error
	_, oldID := previous.Record().Identity()
	if oldID == "" {
		saved, err = r.store.Create(r.ctx, next)
	} else {
		saved, err = r.store.Save(r.ctx, previous, next)
	}
	if err != nil {
		// A definite conflict or rejection cannot authorize merging unknown state.
		var detail *ocistorage.Error
		if errors.As(err, &detail) && detail != nil {
			if detail.Code == ocistorage.Conflict && detail.Status == 412 && !detail.WriteUncertain {
				return ocistorage.StoredNotice{}, &DiscoveryError{Code: "conflict", HTTPStatus: 412}
			}
			rejected := detail.Code == ocistorage.Service && detail.Status >= 400 && detail.Status < 500
			if detail.WriteUncertain && !rejected {
				saved, err = r.confirm(next)
			}
		}
		if err != nil {
			return ocistorage.StoredNotice{}, discoveryFailureFor(r.ctx, "write_failed", err)
		}
	}
	if err := r.guard(); err != nil {
		return ocistorage.StoredNotice{}, err
	}
	return saved, nil
}

func (r *discoveryRun) confirm(want noticeledger.Record) (ocistorage.StoredNotice, error) {
	if err := r.guard(); err != nil {
		return ocistorage.StoredNotice{}, err
	}
	_, id := want.Identity()
	got, err := r.store.Load(r.ctx, id)
	if err != nil {
		return ocistorage.StoredNotice{}, discoveryFailureFor(r.ctx, "confirmation_read", err)
	}
	expected, _ := want.Encode() // The exact proposal was admitted before dispatch.
	actual, _ := got.Record().Encode()
	if !bytes.Equal(expected, actual) {
		return ocistorage.StoredNotice{}, &DiscoveryError{Code: "confirmation_mismatch"}
	}
	return got, nil // This freshly loaded handle owns the next immediate parent.
}
