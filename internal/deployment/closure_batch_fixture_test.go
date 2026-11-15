package deployment

import (
	"testing"

	"oci-maintenance-router/internal/configuration"
	"oci-maintenance-router/internal/delivery"
)

func preparedClosureFixture(t *testing.T) savedReviewFixture {
	t.Helper()
	f := newSavedReviewFixture(t, "ocid1.announcement.oc1..batches", 2, true)
	p, failure := loadPinned(operationalContext(t), f.s, f.objects)
	if failure.Status != "" {
		t.Fatal(failure)
	}
	builder, err := delivery.New(p.configBytes, p.catalog, configuration.LedgerLocation{Namespace: f.s.Namespace, Bucket: f.s.Bucket})
	if err != nil {
		t.Fatal(err)
	}
	// Separate genuine preparations preserve two independent batches in one revision.
	for _, row := range f.record.Snapshot().Revisions[0].Resources {
		result, err := builder.Build(f.notice, false, f.lookups, []string{row.ResourceID})
		if err != nil || len(result.Drafts) != 1 {
			t.Fatal(result, err)
		}
		next, err := f.record.Prepare(reviewTime(), delivery.Preparation{Draft: result.Drafts[0], SecretVersion: 7})
		if err != nil {
			t.Fatal(err)
		}
		f.save(t, next)
	}
	next, err := f.record.MigrateRecovery(reviewTime())
	if err != nil {
		t.Fatal(err)
	}
	f.save(t, next)
	return f
}
