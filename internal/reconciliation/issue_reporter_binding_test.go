package reconciliation

import (
	"context"
	"testing"

	"oci-maintenance-router/internal/ocinotifications"
)

func TestIssueReporterRequiresLiveInputsAndActualChannelClient(t *testing.T) {
	f := issueReporterFixtureFor(t)
	for _, kind := range []string{"objects", "client", "configuration", "mode"} {
		t.Run(kind, func(t *testing.T) {
			var objects PreparationObjects = f.processing.live.objects
			var client *ocinotifications.Client = operatorClient(t)
			config := f.processing.live.config
			switch kind {
			case "objects":
				objects = nil
			case "client":
				client = nil
			case "configuration":
				config = []byte("bad")
			case "mode":
				config = liveSchemaConfig(t, "TEST")
			}
			got, err := NewIssueReporter(config, []byte(preparationCatalog), candidateLocation(), objects, client)
			if err == nil || got != nil {
				t.Fatal("invalid reporting binding accepted")
			}
		})
	}
	d, err := NewIssueReporter(f.processing.live.config, []byte(preparationCatalog), candidateLocation(), f.processing.live.objects, operatorClient(t))
	if err != nil {
		t.Fatal(err)
	}
	// The actual client must reject the synthetic signing principal before I/O.
	if got, err := d.read(leaseBudget(t)); err == nil || got.publish != nil {
		t.Fatal("unsigned channel became a publisher")
	}
	if _, err := (*IssueReporter)(nil).Retry(context.Background(), nil); err == nil {
		t.Fatal("nil reporter accepted")
	}
}
