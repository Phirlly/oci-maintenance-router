package ociannouncements

import (
	"regexp"
	"unicode"
	"unicode/utf8"
)

var regionLabel = regexp.MustCompile(`^[a-z]([a-z0-9-]*[a-z0-9])?$`)
var tenancyID = regexp.MustCompile(`^ocid1\.tenancy\.oc1\.\.([A-Za-z0-9_-]*\.)?[A-Za-z0-9_-]+$`)
var announcementID = regexp.MustCompile(`^ocid1\.announcement\.oc1\.\.([A-Za-z0-9_-]*\.)?[A-Za-z0-9_-]+$`)

func validID(value string, pattern *regexp.Regexp) bool {
	return len(value) <= 4096 && pattern.MatchString(value)
}

func validPageToken(value string) bool {
	if len(value) > 4096 || !utf8.ValidString(value) {
		return false
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}
