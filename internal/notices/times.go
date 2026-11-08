package notices

import (
	"regexp"
	"strings"
	"time"
)

var timestampShape = regexp.MustCompile(
	"^[0-9]{4}-[0-9]{2}-[0-9]{2}T([01][0-9]|2[0-3]):[0-5][0-9]:[0-5][0-9]" +
		"(\\.[0-9]{1,9})?(Z|[+-]([01][0-9]|2[0-3]):[0-5][0-9])$")

func normalizeTimestamp(value *string, field string) (*string, error) {
	if value == nil {
		return nil, nil
	}
	if !timestampShape.MatchString(*value) || strings.HasSuffix(*value, "-00:00") {
		return nil, &Error{field, "invalid_timestamp"}
	}
	parsed, err := time.Parse(time.RFC3339Nano, *value)
	if err != nil || parsed.UTC().Year() < 0 || parsed.UTC().Year() > 9999 {
		return nil, &Error{field, "invalid_timestamp"}
	}
	// Fixed fractional digits truncate finer precision; never round the window.
	normalized := parsed.UTC().Format("2006-01-02T15:04:05.000Z")
	return &normalized, nil
}

func normalizeTimes(record *Record) error {
	for _, field := range []struct {
		name   string
		target **string
	}{
		{"times[0].value", &record.Times[0].Value},
		{"times[1].value", &record.Times[1].Value},
		{"timeCreated", &record.TimeCreated},
		{"timeUpdated", &record.TimeUpdated},
	} {
		value, err := normalizeTimestamp(*field.target, field.name)
		if err != nil {
			return err
		}
		*field.target = value
	}
	return nil
}
