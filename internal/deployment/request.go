package deployment

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"unicode/utf8"
)

type command struct {
	operation, configurationSHA256, releaseSHA256, attemptID string
}

// Commands select work only. All operational authority comes from pinned settings.
func parseCommand(body []byte) (command, error) {
	invalid := errors.New("invalid command")
	if !utf8.Valid(body) {
		return command{}, invalid
	}
	d := json.NewDecoder(bytes.NewReader(body))
	start, err := d.Token()
	if err != nil || start != json.Delim('{') {
		return command{}, invalid
	}
	fields := map[string]string{}
	for d.More() {
		token, err := d.Token()
		if err != nil {
			return command{}, invalid
		}
		name, ok := token.(string)
		if _, exists := fields[name]; !ok || exists {
			return command{}, invalid
		}
		switch name {
		case "operation", "configurationSha256", "releaseSha256", "checkAttemptId":
		default:
			return command{}, invalid
		}
		value, err := d.Token()
		text, ok := value.(string)
		if err != nil || !ok {
			return command{}, invalid
		}
		fields[name] = text
	}
	end, err := d.Token()
	if err != nil || end != json.Delim('}') || d.Decode(new(any)) != io.EOF {
		return command{}, invalid
	}
	c := command{fields["operation"], fields["configurationSha256"], fields["releaseSha256"], fields["checkAttemptId"]}
	switch c.operation {
	case "deployment-check":
		if len(fields) != 4 || !attemptPattern.MatchString(c.attemptID) || !hashPattern.MatchString(c.configurationSHA256) || !hashPattern.MatchString(c.releaseSHA256) {
			return command{}, invalid
		}
	case "reconcile":
		if len(fields) != 1 {
			return command{}, invalid
		}
	default:
		return command{}, invalid
	}
	return c, nil
}
