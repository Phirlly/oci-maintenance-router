package deployment

import (
	"net/http"
	"strings"
	"testing"
)

func TestRejectedInputLogUsesOnlyFixedMetadataShapes(t *testing.T) {
	for _, prefix := range []string{"", "Fn-Http-H-"} {
		for _, tc := range []struct {
			header, field string
			limit         int
		}{
			{"TopicOcid", "topic", 1024}, {"MessageId", "messageId", 1024}, {"MessageType", "messageType", 128},
		} {
			for _, kind := range []string{"absent", "empty", "nil", "valid", "multiple", "duplicate-case", "space", "oversized", "control", "utf8"} {
				t.Run(prefix+tc.header+"/"+kind, func(t *testing.T) {
					headers := http.Header{"X-OCI-NS-Private": {"https://private.invalid/secret"}, "Fn-Http-H-X-OCI-NS-Private": {"private-signature"}}
					key := prefix + "X-OCI-NS-" + tc.header
					want := "invalid"
					switch kind {
					case "absent":
						want = "absent"
					case "empty":
						headers.Set(key, "")
					case "nil":
						headers[key] = nil
					case "valid":
						headers.Set(key, strings.Repeat("v", tc.limit))
						want = "valid"
					case "multiple":
						headers[key] = []string{"private-value", "private-value"}
						want = "multiple"
					case "duplicate-case":
						headers[key] = nil
						headers[strings.ToLower(key)] = []string{"private-value"}
						want = "multiple"
					case "space":
						headers.Set(key, " ")
					case "oversized":
						headers.Set(key, strings.Repeat("v", tc.limit+1))
					case "control":
						headers.Set(key, "private\nvalue")
					case "utf8":
						headers.Set(key, "private\xff")
					}
					// A valid MessageId alone selects notifications. Reject its body
					// independently so this test still observes the metadata shape.
					body := `{"operation":"reconcile"}`
					if prefix == "" && tc.header == "MessageId" && kind == "valid" {
						body = strings.Repeat("x", 65537)
					}
					got := rejectedInputLog(t, headers, strings.NewReader(body))
					if prefix == "" && tc.header == "MessageId" && kind == "valid" && got["category"] != "notification_body_size" {
						t.Fatal("valid MessageId rejected before the bounded body", got)
					}
					group, other := "direct", "prefixed"
					if prefix != "" {
						group, other = other, group
					}
					shape := got[group].(map[string]any)
					if len(shape) != 3 || shape[tc.field] != want || got["directMetadata"] != true || got["prefixedMetadata"] != true {
						t.Fatalf("incorrect bounded shape: %v", got)
					}
					for _, value := range got[other].(map[string]any) {
						if value != "absent" {
							t.Fatal("diagnostic mixed header representations", got)
						}
					}
				})
			}
		}
	}
}

func TestRejectedInputLogExcludesUnrelatedHeaders(t *testing.T) {
	headers := http.Header{"Authorization": {"private-key"}, "Private-Arbitrary-Name": {"private-value"}}
	got := rejectedInputLog(t, headers, strings.NewReader("private-body"))
	if got["directMetadata"] != false || got["prefixedMetadata"] != false || got["category"] != "command_invalid" {
		t.Fatal("unrelated headers changed diagnostic", got)
	}
	for _, group := range []string{"direct", "prefixed"} {
		for _, value := range got[group].(map[string]any) {
			if value != "absent" {
				t.Fatal("unrelated headers reported as ONS", got)
			}
		}
	}
}
