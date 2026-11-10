package reconciliation

import (
	"context"
	"testing"

	"oci-maintenance-router/internal/noticeledger"
	"oci-maintenance-router/internal/notices"
	"oci-maintenance-router/internal/ociannouncements"
)

func TestSourceAvailabilityIsOwnedAndDistinguishesZeroFromEmpty(t *testing.T) {
	a, s := detailSources(t, nil, nil)
	empty, e := collector(t, a, s).collect(budget(t), noPause)
	if e != nil || empty.Availability.EnvironmentID() == "" || empty.Availability.Unavailable() == nil || len(empty.Availability.Unavailable()) != 0 {
		t.Fatal("missing successful empty availability", e)
	}
	var zero SourceAvailability
	if zero.EnvironmentID() != "" || zero.Unavailable() != nil {
		t.Fatal("zero availability usable")
	}
	n := candidateNotice(t, 1, nil, true)
	r := candidateRecord(t, n)
	a, s = detailSources(t, nil, []noticeledger.Record{r})
	a.get = func(context.Context, string) (notices.Notice, error) {
		return notices.Notice{}, &ociannouncements.Error{Code: ociannouncements.Service, Status: 404}
	}
	s.load = func(context.Context, string) (noticeledger.Record, error) { return r, nil }
	c := collector(t, a, s)
	for i := 0; i < 8; i++ {
		t.Run(noticeID(i), func(t *testing.T) {
			t.Parallel()
			got, e := c.collect(budget(t), noPause)
			if e != nil || len(got.Availability.Unavailable()) != 1 {
				t.Fatal("lost independent outcome", e)
			}
			view := got.Availability.Unavailable()
			view[0].NoticeID = "changed"
			view[0].HTTPStatus = 200
			if again := got.Availability.Unavailable(); again[0].NoticeID != n.Key().NoticeID || again[0].HTTPStatus != 404 {
				t.Fatal("snapshot mutation changed retained evidence")
			}
		})
	}
}
