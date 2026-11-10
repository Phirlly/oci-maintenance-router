package reconciliation

import (
	"context"
	"time"
)

// Begin once before listing; every later discovery stage inherits this context.
func discoveryContext(ctx context.Context) (context.Context, context.CancelFunc, *Error) {
	if ctx == nil {
		return nil, nil, &Error{Code: InvalidRequest}
	}
	if ctx.Err() != nil {
		return nil, nil, failure(ctx, "", Canceled)
	}
	if _, ok := ctx.Deadline(); !ok {
		return nil, nil, &Error{Code: DeadlineRequired}
	}
	bounded, cancel := context.WithTimeout(ctx, 120*time.Second)
	return bounded, cancel, nil
}
