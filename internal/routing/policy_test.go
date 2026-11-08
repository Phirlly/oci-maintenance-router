package routing_test

import (
	"errors"
	"reflect"
	"testing"

	"oci-maintenance-router/internal/configuration"
	"oci-maintenance-router/internal/notices"
	"oci-maintenance-router/internal/routing"
)

func TestPolicyRejectsUntrustedOrAmbiguousSettings(t *testing.T) {
	for _, tc := range []struct {
		name    string
		data    []byte
		catalog configuration.Catalog
	}{
		{"bad json", []byte(`{"private-input":true}`), catalog()},
		{"no catalog", settings(t, nil), configuration.Catalog{}},
		{"alias collision", settings(t, nil), configuration.Catalog{
			Regions:       map[string]string{"us-ashburn-1": "iad", "iad": "other"},
			MySQLServices: catalog().MySQLServices}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p, err := routing.New(tc.data, tc.catalog)
			var controlled *configuration.Error
			if p != nil || !errors.As(err, &controlled) {
				t.Fatalf("expected controlled error and no policy: %v %v", p, err)
			}
		})
	}
}

func TestPolicyOwnsConfigurationAndCatalog(t *testing.T) {
	data, cat := settings(t, nil), catalog()
	c, err := configuration.Decode(data, cat)
	if err != nil {
		t.Fatal(err)
	}
	wantGeneration, err := c.RoutingGeneration()
	if err != nil {
		t.Fatal(err)
	}
	p, err := routing.New(data, cat)
	if err != nil {
		t.Fatal(err)
	}
	for i := range data {
		data[i] = 'x'
	}
	cat.Regions["us-ashburn-1"] = "changed"
	delete(cat.Regions, "us-phoenix-1")
	cat.MySQLServices[0] = "changed"
	n := notice(t, record(db))
	assessment, targets := p.Targets(n, false)
	if assessment.State != routing.SupportedShape || !reflect.DeepEqual(targets, []routing.Target{{ResourceID: db, Region: "us-ashburn-1"}}) {
		t.Fatalf("caller mutation changed policy: %+v %+v", assessment, targets)
	}
	got := p.Resolve(n, false, map[string]routing.Lookup{db: metadata(db, "payments")})
	if got.Revision != n.Key() || got.RoutingGeneration != wantGeneration || len(got.Groups) != 1 || got.Groups[0].DestinationID != "payments" {
		t.Fatalf("lost identity or routing configuration: %+v", got)
	}
	other := notice(t, record("ocid1.mysqldbsystem.oc1.phx.example"))
	if got := p.Resolve(other, false, nil); len(got.Decisions) != 1 || got.Decisions[0].Disposition != routing.OutOfScope {
		t.Fatalf("full known catalog was lost: %+v", got)
	}
}

func TestZeroPolicyAndNoticeStayInReview(t *testing.T) {
	for _, p := range []*routing.Policy{nil, {}, policy(t, "defined")} {
		n := notices.Notice{}
		if p == nil {
			n = notice(t, record(db))
		}
		a, targets := p.Targets(n, false)
		got := p.Resolve(n, false, map[string]routing.Lookup{db: metadata(db, "payments")})
		if a.State != routing.ReviewRequired || got.Assessment != a || len(targets) != 0 || len(got.Groups) != 0 {
			t.Fatalf("zero value authorized work: %+v %+v %+v", a, targets, got)
		}
	}
}
