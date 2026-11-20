package reconciliation

import (
	"bytes"
	"context"
	"reflect"
	"regexp"
	"time"

	"oci-maintenance-router/internal/configuration"
	"oci-maintenance-router/internal/delivery"
	"oci-maintenance-router/internal/noticeledger"
	"oci-maintenance-router/internal/ocimysql"
	"oci-maintenance-router/internal/routing"
)

// RecoveryReview is owned observed evidence, never persisted state or authority.
// Saved retains historical clocks; Projected uses those clocks only to calculate
// the existing action fingerprint. Routing shows current prospective ownership.
type RecoveryReview struct {
	Saved, Projected     noticeledger.Record
	Routing              routing.Plan
	Component            CandidateComponent
	Fingerprint, Blocker string
}

type RecoveryReviewError struct{ Code string }

func (e *RecoveryReviewError) Error() string { return "recovery review: " + e.Code }

// RecoveryReviewer deliberately accepts only a read-only saved-record interface.
// The caller owns CURRENT checks and worker coordination; this cannot save/send.
type RecoveryReviewer struct {
	config    configuration.Config
	collector *Collector
	observer  *noticeledger.Observer
	policy    *routing.Policy
	builder   *delivery.Builder
	mysql     map[string]func(context.Context, string) (routing.Metadata, error)
	wait      func(context.Context, time.Duration) error
}

var recoveryReviewNoticeID = regexp.MustCompile(`^ocid1\.announcement\.oc1\.\.([A-Za-z0-9_-]*\.)?[A-Za-z0-9_-]+$`)

func NewRecoveryReviewer(data, catalogBytes []byte, location configuration.LedgerLocation, source AnnouncementSource, saved SavedNoticeSource, mysql map[string]*ocimysql.Client) (*RecoveryReviewer, error) {
	invalid := &RecoveryReviewError{Code: "invalid_request"}
	for _, source := range []any{source, saved} {
		if source == nil {
			return nil, invalid
		}
		v := reflect.ValueOf(source)
		switch v.Kind() {
		case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
			if v.IsNil() {
				return nil, invalid
			}
		}
	}
	data, catalogBytes = bytes.Clone(data), bytes.Clone(catalogBytes)
	catalog, err := configuration.DecodeCatalog(catalogBytes)
	if err != nil {
		return nil, invalid
	}
	c, err := configuration.Decode(data, catalog)
	if err != nil || (c.SchemaVersion != 7 && c.SchemaVersion != 8) || c.Mode != "LIVE" {
		return nil, invalid
	}
	collector, err := NewCollector(data, catalog, location, source, saved)
	if err != nil {
		return nil, invalid
	}
	lookups := make(map[string]func(context.Context, string) (routing.Metadata, error), len(c.RegionMap))
	for region := range c.RegionMap {
		client := mysql[region]
		if client == nil {
			return nil, invalid
		}
		lookups[region] = client.Get
	}
	observer, _ := noticeledger.New(data, catalog, location)
	policy, _ := routing.New(data, catalog)
	builder, _ := delivery.New(data, catalog, location)
	return &RecoveryReviewer{c, collector, observer, policy, builder, lookups, pause}, nil
}

func reviewFailure(ctx context.Context, code string) error {
	if ctx != nil && ctx.Err() != nil {
		code = "canceled"
		if ctx.Err() == context.DeadlineExceeded {
			code = "deadline_exceeded"
		}
	}
	return &RecoveryReviewError{Code: code}
}
