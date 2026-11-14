package reconciliation

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"oci-maintenance-router/internal/configuration"
	"oci-maintenance-router/internal/noticeledger"
	"oci-maintenance-router/internal/notices"
)

const baselineCompartment = "ocid1.compartment.oc1..example"
const baselineOtherCompartment = "ocid1.compartment.oc1..other"

func baselineTime() time.Time { return time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC) }

func baselineResult(t *testing.T, ns []notices.Notice, records []noticeledger.Record, compartments, regions []string, changes ...func(map[string]any)) DiscoveryResult {
	t.Helper()
	d, s, lease := discoveryFixture(t, ns, records)
	var config map[string]any
	if err := json.Unmarshal([]byte(candidateConfig), &config); err != nil {
		t.Fatal(err)
	}
	if compartments != nil {
		config["compartmentIds"] = compartments
	}
	catalog := configuration.Catalog{Regions: map[string]string{"us-ashburn-1": "iad", "us-phoenix-1": "phx"}, MySQLServices: []string{"Oracle HeatWave MySQL"}}
	if regions != nil {
		selected := map[string]string{}
		for _, r := range regions {
			selected[r] = catalog.Regions[r]
		}
		config["regionMap"] = selected
	}
	for _, change := range changes {
		change(config)
	}
	configBytes, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	catalogBytes, err := json.Marshal(map[string]any{"regions": catalog.Regions, "mysqlServices": catalog.MySQLServices})
	if err != nil {
		t.Fatal(err)
	}
	s.seed("configuration/router.json", configBytes)
	s.seed("configuration/catalog.json", catalogBytes)
	next, err := NewDiscovery(configBytes, catalogBytes, candidateLocation(), d.collector.announcements, s)
	if err != nil {
		t.Fatal(err)
	}
	next.wait = d.wait
	next.now = baselineTime
	result, err := next.Run(leaseBudget(t), lease)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func buildBaseline(t *testing.T, id string, r DiscoveryResult) Baseline {
	t.Helper()
	b, err := r.Baseline(id)
	if err != nil {
		t.Fatal("successful discovery could not propose baseline", err)
	}
	return b
}
func baselineFailure(t *testing.T, b Baseline, err error, code string) {
	t.Helper()
	var detail *BaselineError
	if !errors.As(err, &detail) || detail.Code != code {
		t.Fatalf("error=%v; want %s", err, code)
	}
	if b.ID() != "" || b.EnvironmentID() != "" || !b.CompletedAt().IsZero() || b.Layers() != nil {
		t.Fatal("failure returned partial baseline")
	}
}
