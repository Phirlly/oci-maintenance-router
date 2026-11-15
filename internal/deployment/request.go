package deployment

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"regexp"
	"unicode/utf8"
)

type command struct {
	operation, configurationSHA256, releaseSHA256, attemptID string
	noticeID, cursor                                         string
	kind, targetID, recoveryRequestID                        string
	automatic                                                bool
	intakeTopic                                              string
}

var reviewNoticeID = regexp.MustCompile(`^ocid1\.announcement\.oc1\.\.([A-Za-z0-9_-]*\.)?[A-Za-z0-9_-]+$`)

// Commands select work only. All operational authority comes from pinned settings.
func parseCommand(body []byte) (command, error) {
	invalid := errors.New("invalid command")
	if len(body) > 8192 || !utf8.Valid(body) {
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
		case "operation", "configurationSha256", "releaseSha256", "checkAttemptId", "noticeId", "cursor", "kind", "targetId", "recoveryRequestId":
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
	c := command{operation: fields["operation"], configurationSHA256: fields["configurationSha256"], releaseSHA256: fields["releaseSha256"], attemptID: fields["checkAttemptId"], noticeID: fields["noticeId"], cursor: fields["cursor"]}
	c.kind, c.targetID, c.recoveryRequestID = fields["kind"], fields["targetId"], fields["recoveryRequestId"]
	if c.operation != "review-notice" && c.operation != "review-closure" && len(body) > 1024 {
		return command{}, invalid
	}
	switch c.operation {
	case "deployment-check":
		if len(fields) != 4 || !attemptPattern.MatchString(c.attemptID) || !hashPattern.MatchString(c.configurationSHA256) || !hashPattern.MatchString(c.releaseSHA256) {
			return command{}, invalid
		}
	case "close-work":
		if len(fields) != 5 || !attemptPattern.MatchString(c.attemptID) || !hashPattern.MatchString(c.configurationSHA256) || !hashPattern.MatchString(c.releaseSHA256) || !closureRequestIDPattern.MatchString(c.recoveryRequestID) {
			return command{}, invalid
		}
	case "reconcile", "wake-up":
		if len(fields) != 1 {
			return command{}, invalid
		}
		if c.operation == "wake-up" {
			c.operation, c.automatic = "reconcile", true
		}
	case "list-notices":
		for name := range fields {
			if name != "operation" && name != "cursor" {
				return command{}, invalid
			}
		}
		if _, err := parseListingCursor(c.cursor); err != nil {
			return command{}, invalid
		}
	case "review-notice", "review-closure":
		for name := range fields {
			if c.operation == "review-closure" && (name == "kind" || name == "targetId") {
				continue
			}
			if name != "operation" && name != "noticeId" && name != "cursor" {
				return command{}, invalid
			}
		}
		if len(c.noticeID) > 4096 || !reviewNoticeID.MatchString(c.noticeID) {
			return command{}, invalid
		}
		if _, _, err := parseReviewCursor(c.cursor); err != nil {
			return command{}, invalid
		}
		_, kindPresent := fields["kind"]
		_, targetPresent := fields["targetId"]
		if kindPresent != targetPresent || kindPresent && (c.kind != "CLOSE_BATCH" && c.kind != "CLOSE_REVISION" || !hashPattern.MatchString(c.targetID)) {
			return command{}, invalid
		}
	default:
		return command{}, invalid
	}
	return c, nil
}
