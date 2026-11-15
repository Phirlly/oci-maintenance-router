package reconciliation

import (
	"context"
	"slices"

	"oci-maintenance-router/internal/ocistorage"
)

// The current observed page is consumed before another listing. Changing live
// membership cannot reset progress. A sweep is not an inventory snapshot.
func nextReportNotice(ctx context.Context, store *ocistorage.NoticeStore, c *reportCursor) (string, bool, error) {
	if len(c.Names) == 0 {
		page, err := store.ListPage(ctx, c.Start)
		if err != nil {
			return "", false, &ReportingError{"listing_failed"}
		}
		slices.Sort(page.Names)
		candidate := reportCursor{Start: c.Start, Names: page.Names, Next: page.NextStartWith}
		if !validReportCursor(candidate) {
			return "", false, &ReportingError{"listing_invalid"}
		}
		*c = candidate
	}
	name := ""
	if c.Offset < len(c.Names) {
		name = c.Names[c.Offset]
		c.Offset++
	}
	if c.Offset == len(c.Names) {
		done := c.Next == ""
		*c = reportCursor{Start: c.Next, Names: []string{}}
		return name, done, nil
	}
	return name, false, nil
}
