package reconciliation

import (
	"bytes"
	"context"
	"oci-maintenance-router/internal/noticeledger"
	"oci-maintenance-router/internal/ocistorage"
)

func sameNotice(a, b noticeledger.Record) bool {
	x, _ := a.Encode()
	y, _ := b.Encode()
	return bytes.Equal(x, y)
}

func (r *processingRun) save(previous ocistorage.StoredNotice, next noticeledger.Record) (ocistorage.StoredNotice, error) {
	if err := r.guard(); err != nil {
		return ocistorage.StoredNotice{}, err
	}
	if sameNotice(previous.Record(), next) {
		return previous, nil
	}
	got, err := r.write(r.ctx, previous, next)
	if err == nil {
		err = r.guard()
	}
	return got, err
}

// write is shared only by guarded transitions and the exact already-issued
// outcome. It never rebases a proposal onto a newly loaded predecessor.
func (r *processingRun) write(ctx context.Context, previous ocistorage.StoredNotice, next noticeledger.Record) (ocistorage.StoredNotice, error) {
	got, err := r.store.Save(ctx, previous, next)
	if err == nil {
		return got, nil
	}
	e, ok := err.(*ocistorage.Error)
	if !ok || e == nil || !e.WriteUncertain || (e.Code == ocistorage.Service && e.Status >= 400 && e.Status < 500) {
		return ocistorage.StoredNotice{}, processingFailure(ctx, err)
	}
	_, id := next.Identity()
	got, err = r.store.Load(ctx, id)
	if err != nil {
		return ocistorage.StoredNotice{}, &ProcessingError{Code: "confirmation_failed", WriteUncertain: true}
	}
	if !sameNotice(got.Record(), next) {
		return ocistorage.StoredNotice{}, &ProcessingError{Code: "confirmation_mismatch", WriteUncertain: true}
	}
	return got, nil
}
