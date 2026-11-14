package ocistorage

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestNoticeBackendErrorsReturnNoPartialSuccessOrRawDetails(t *testing.T) {
	for _, op := range noticeOperations {
		t.Run(op, func(t *testing.T) {
			write := op == "create" || op == "save"
			for _, tt := range []struct {
				err       error
				code      Code
				status    int
				uncertain bool
			}{
				{errors.New("private backend detail"), Transport, 0, write},
				{&Error{Code: "private backend detail"}, Transport, 0, write},
				{(*Error)(nil), Transport, 0, write},
				{&Error{Code: Service, Status: 404, WriteUncertain: write}, Service, 404, write},
				{&Error{Code: Conflict, Status: 412}, Conflict, 412, false},
				{&Error{Code: Transport, WriteUncertain: write}, Transport, 0, write},
				{&Error{Code: Authentication}, Authentication, 0, false},
				{&Error{Code: InvalidResponse, Status: 200, WriteUncertain: write}, InvalidResponse, 200, write},
			} {
				b := &noticeBackend{}
				invoke := backendOperation(t, b, op)
				calls := 0
				backendResponse(t, b, func(context.Context) error { calls++; return tt.err })
				err := invoke(budget(t))
				wantError(t, err, tt.code, tt.status, tt.uncertain)
				if strings.Contains(err.Error(), "private") || calls != 1 {
					t.Fatal("raw detail or retry escaped")
				}
			}
		})
	}
}

func TestNoticeBackendValidatesReadBodyAndVersion(t *testing.T) {
	r := noticeState(t)
	body, _ := r.Encode()
	env, _ := r.Identity()
	for _, op := range []string{"load", "load-object"} {
		for _, tt := range []struct {
			name string
			body []byte
			tag  string
			code Code
		}{
			{"etag", body, "W/weak", InvalidResponse},
			{"oversize", make([]byte, objectLimit+1), "tag", TooLarge},
			{"corrupt", []byte("private malformed"), "tag", InvalidResponse},
			{"canonical", append(append([]byte(nil), body...), ' '), "tag", InvalidResponse},
			{"identity", []byte(strings.ReplaceAll(string(body), stateNoticeID, stateNoticeID+"other")), "tag", InvalidResponse},
			{"environment", []byte(strings.ReplaceAll(string(body), env, strings.Repeat("f", 64))), "tag", InvalidResponse},
		} {
			t.Run(op+"/"+tt.name, func(t *testing.T) {
				b := &noticeBackend{}
				invoke := backendOperation(t, b, op)
				b.get = func(context.Context, string) (Object, error) { return Object{tt.body, tt.tag}, nil }
				wantError(t, invoke(budget(t)), tt.code, 200, false)
			})
		}
	}
}

func TestNoticeBackendInvalidWriteVersionRemainsUncertain(t *testing.T) {
	for _, op := range []string{"create", "save"} {
		for _, tag := range []string{"", "*", "a,b", "W/weak", "private\r\nheader"} {
			b := &noticeBackend{}
			invoke := backendOperation(t, b, op)
			b.create = func(context.Context, string, []byte) (string, error) { return tag, nil }
			b.replace = func(context.Context, string, string, []byte) (string, error) { return tag, nil }
			wantError(t, invoke(budget(t)), InvalidResponse, 200, true)
		}
	}
}
