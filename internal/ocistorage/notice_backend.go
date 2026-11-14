package ocistorage

import (
	"context"
	"reflect"
)

// NoticeObjects supplies conditional byte storage and current notice names.
// Implementations must honor context deadlines, return complete owned results,
// and never retry writes. Create requires absence; Replace requires the exact
// supplied ETag. Bootstrap binds the implementation to the store's environment.
type NoticeObjects interface {
	Get(context.Context, string) (Object, error)
	Create(context.Context, string, []byte) (string, error)
	Replace(context.Context, string, string, []byte) (string, error)
	ListNoticePage(context.Context, string) (NoticePage, error)
}

var _ NoticeObjects = (*Client)(nil)

func usableNoticeObjects(objects NoticeObjects) bool {
	if objects == nil {
		return false
	}
	v := reflect.ValueOf(objects)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		if v.IsNil() {
			return false
		}
	}
	if client, ok := objects.(*Client); ok {
		return client.signer != nil && client.transport != nil
	}
	return true
}

func noticeContext(ctx context.Context) (context.Context, context.CancelFunc, error) {
	if ctx == nil {
		return nil, nil, &Error{Code: InvalidRequest}
	}
	if ctx.Err() != nil {
		return nil, nil, failure(ctx, Canceled, 0, false)
	}
	if _, ok := ctx.Deadline(); !ok {
		return nil, nil, &Error{Code: DeadlineRequired}
	}
	bounded, cancel := context.WithTimeout(ctx, operationLimit)
	return bounded, cancel, nil
}

// Never pass through arbitrary backend error text, including a forged Code.
// Unknown/wrapped failures cannot establish a definite write disposition.
func noticeBackendFailure(ctx context.Context, err error, write bool) error {
	if e, ok := err.(*Error); ok && e != nil {
		switch e.Code {
		case InvalidConfiguration, InvalidRequest, DeadlineRequired, Canceled,
			DeadlineExceeded, Authentication, Transport, InvalidResponse, TooLarge, Conflict, Service:
			return failure(ctx, e.Code, e.Status, e.WriteUncertain)
		}
	}
	return failure(ctx, Transport, 0, write)
}
