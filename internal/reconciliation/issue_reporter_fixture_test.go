package reconciliation

import (
	"bytes"
	"context"
	"testing"

	"oci-maintenance-router/internal/ocinotifications"
)

type issueReporterFixture struct {
	processing *processingFixture
	reporter   *IssueReporter
	snapshot   ProcessingSnapshot
	sends      int
	payloads   [][]byte
}

func issueReporterFixtureFor(t *testing.T) *issueReporterFixture {
	t.Helper()
	f := &issueReporterFixture{processing: processingFixtureFor(t, processingNotice(t, 1, processingDB))}
	p := f.processing
	p.p.current = func(context.Context, string) (maintenanceCredential, error) { return nil, nil }
	f.snapshot = p.run(t)
	var err error
	f.reporter, err = NewIssueReporter(p.live.config, []byte(preparationCatalog), candidateLocation(), p.live.objects, operatorClient(t))
	if err != nil {
		t.Fatal(err)
	}
	f.reporter.proof.read = p.live.selector.operator.read
	f.reporter.read = func(context.Context) (operatorObservation, error) {
		return operatorObservation{f.reporter.desired, func(ctx context.Context, body []byte) (ocinotifications.Publication, error) {
			f.sends++
			s := f.saved(t)
			if s.Pending == nil || s.Pending.Claim == "" || !bytes.Equal(s.Pending.Payload, body) || ctx.Err() != nil {
				t.Error("publication lacked the exact confirmed claim")
			}
			f.payloads = append(f.payloads, bytes.Clone(body))
			return ocinotifications.Publication{Status: 202, MessageID: "operator-message", MayHaveSent: true}, nil
		}}, nil
	}
	return f
}

func (f *issueReporterFixture) saved(t *testing.T) reportState {
	t.Helper()
	o := f.processing.live.objects
	o.mu.Lock()
	body := bytes.Clone(o.objects[issueReportName].Body)
	o.mu.Unlock()
	s, err := decodeReport(body, f.reporter.environment, f.reporter.installation)
	if err != nil {
		t.Fatal(err)
	}
	return s
}
