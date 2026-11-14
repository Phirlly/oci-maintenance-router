package ocinotifications

import (
	"net/url"
	"regexp"
	"strings"
)

var dnsLabel = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`)

// Only a returned origin inside this configured OC1 regional service is usable.
// Cell names are never synthesized. No arbitrary redirects or URL overrides exist.
func (c *Client) topicOrigin(raw string) (string, bool) {
	if raw == "" || len(raw) > 2048 {
		return "", false
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Port() != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || (u.Path != "" && u.Path != "/") || raw != "https://"+u.Host+u.Path {
		return "", false
	}
	modern := "notification." + c.region + ".oci.oraclecloud.com"
	legacy := "notification." + c.region + ".oraclecloud.com"
	if u.Host != legacy && u.Host != modern && !strings.HasSuffix(u.Host, "."+modern) {
		return "", false
	}
	if len(u.Host) > 253 {
		return "", false
	}
	for _, label := range strings.Split(u.Host, ".") {
		if len(label) > 63 || !dnsLabel.MatchString(label) {
			return "", false
		}
	}
	return "https://" + u.Host, true
}
