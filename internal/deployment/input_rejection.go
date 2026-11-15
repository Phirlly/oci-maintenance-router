package deployment

import (
	"net/http"
	"strings"
	"unicode"
	"unicode/utf8"

	fdk "github.com/fnproject/fdk-go"
)

// Input failures carry only a parser-owned category, never an underlying error.
type inputFailure string

type inputRejection struct {
	Category         inputFailure      `json:"category"`
	HTTPContext      bool              `json:"httpContext"`
	DirectMetadata   bool              `json:"directMetadata"`
	PrefixedMetadata bool              `json:"prefixedMetadata"`
	Direct           notificationShape `json:"direct"`
	Prefixed         notificationShape `json:"prefixed"`
}

type notificationShape struct {
	Topic       string `json:"topic"`
	MessageID   string `json:"messageId"`
	MessageType string `json:"messageType"`
}

// Report only what the FDK exposed. This cannot reconstruct upstream headers,
// authorize input, or change parsing. Unknown header names are never retained.
func describeInputRejection(ctx fdk.Context, category inputFailure) *inputRejection {
	_, isHTTP := ctx.(fdk.HTTPContext)
	headers := ctx.Header()
	direct, directPresent := describeNotificationHeaders(headers, "x-oci-ns-")
	prefixed, prefixedPresent := describeNotificationHeaders(headers, "fn-http-h-x-oci-ns-")
	return &inputRejection{Category: category, HTTPContext: isHTTP, DirectMetadata: directPresent,
		PrefixedMetadata: prefixedPresent, Direct: direct, Prefixed: prefixed}
}

func describeNotificationHeaders(headers http.Header, prefix string) (notificationShape, bool) {
	shape := notificationShape{Topic: "absent", MessageID: "absent", MessageType: "absent"}
	present := false
	for key, values := range headers {
		name := strings.ToLower(key)
		if !strings.HasPrefix(name, prefix) {
			continue
		}
		present = true
		var field *string
		limit := 1024
		switch strings.TrimPrefix(name, prefix) {
		case "topicocid":
			field = &shape.Topic
		case "messageid":
			field = &shape.MessageID
		case "messagetype":
			field, limit = &shape.MessageType, 128
		default:
			continue
		}
		if *field != "absent" || len(values) > 1 {
			*field = "multiple"
			continue
		}
		*field = "invalid"
		if len(values) == 1 && len(values[0]) <= limit && strings.TrimSpace(values[0]) != "" && utf8.ValidString(values[0]) && strings.IndexFunc(values[0], unicode.IsControl) < 0 {
			*field = "valid"
		}
	}
	return shape, present
}
