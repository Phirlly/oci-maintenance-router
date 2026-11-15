package deployment

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"unicode"
	"unicode/utf8"
)

// ONS input is a wake-up, never a source of business data or operator commands.
// Header metadata selects a parser; IAM and pinned configuration own authority.
func readCommand(in io.Reader, headers http.Header) (command, inputFailure) {
	topic, notification, err := notificationTopic(headers)
	if err != nil {
		return command{}, "metadata_invalid"
	}
	if notification {
		n, err := io.Copy(io.Discard, io.LimitReader(in, 65537))
		if err != nil {
			return command{}, "notification_body_read"
		}
		if n > 65536 {
			return command{}, "notification_body_size"
		}
		return command{operation: "reconcile", automatic: true, intakeTopic: topic}, ""
	}
	body, err := io.ReadAll(io.LimitReader(in, 8193))
	if err != nil {
		return command{}, "command_body_read"
	}
	if len(body) > 8192 {
		return command{}, "command_body_size"
	}
	request, err := parseCommand(body)
	if err != nil {
		return command{}, "command_invalid"
	}
	return request, ""
}

func notificationTopic(headers http.Header) (string, bool, error) {
	invalid := errors.New("invalid notification metadata")
	values := map[string]string{}
	present := false
	for key, list := range headers {
		name := strings.ToLower(key)
		if !strings.HasPrefix(name, "x-oci-ns-") {
			continue
		}
		present = true
		limit := 1024
		switch name {
		case "x-oci-ns-topicocid", "x-oci-ns-messageid":
		case "x-oci-ns-messagetype":
			limit = 128
		default:
			continue // Never follow or log metadata URLs, signatures or body text.
		}
		if _, exists := values[name]; exists || len(list) != 1 {
			return "", true, invalid
		}
		value := list[0]
		if strings.TrimSpace(value) == "" || len(value) > limit || !utf8.ValidString(value) || strings.IndexFunc(value, unicode.IsControl) >= 0 {
			return "", true, invalid
		}
		values[name] = value
	}
	if present && len(values) != 3 {
		return "", true, invalid
	}
	return values["x-oci-ns-topicocid"], present, nil
}
