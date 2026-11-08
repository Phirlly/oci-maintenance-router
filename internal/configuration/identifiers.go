package configuration

import (
	"regexp"
	"strings"
)

var (
	namePattern      = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,31}$`)
	regionPattern    = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
	regionKeyPattern = regexp.MustCompile(`^[a-z0-9]+$`)
	componentPattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)
	baselinePattern  = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)
	digestPattern    = regexp.MustCompile(`^[a-f0-9]{64}$`)
	bucketPattern    = regexp.MustCompile(`^[A-Za-z0-9._-]{1,256}$`)
)

type identifier struct {
	region string
	future string
	unique string
}

func parseOCID(value, kind string) (identifier, bool) {
	parts := strings.Split(value, ".")
	if (len(parts) != 5 && len(parts) != 6) ||
		parts[0] != "ocid1" || parts[1] != kind || parts[2] != "oc1" {
		return identifier{}, false
	}
	id := identifier{region: parts[3], unique: parts[len(parts)-1]}
	if len(parts) == 6 {
		id.future = parts[4]
	}
	if !componentPattern.MatchString(id.unique) ||
		(id.future != "" && !componentPattern.MatchString(id.future)) {
		return identifier{}, false
	}
	return id, true
}

func globalOCID(value, kind string) bool {
	id, ok := parseOCID(value, kind)
	return ok && id.region == ""
}

func regionalOCID(value, kind, region, key string) (identifier, bool) {
	id, ok := parseOCID(value, kind)
	if !ok || (id.region != region && id.region != key) {
		return identifier{}, false
	}
	id.region = region
	return id, true
}
