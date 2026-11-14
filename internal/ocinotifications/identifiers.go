package ocinotifications

import "regexp"

var regionLabel = regexp.MustCompile(`^[a-z]([a-z0-9-]*[a-z0-9])?$`)
var regionKeyLabel = regexp.MustCompile(`^[a-z0-9]+$`)
var tenancyPattern = regexp.MustCompile(`^ocid1\.tenancy\.oc1\.\.([A-Za-z0-9_-]*\.)?[A-Za-z0-9_-]+$`)
var compartmentPattern = regexp.MustCompile(`^ocid1\.compartment\.oc1\.\.([A-Za-z0-9_-]*\.)?[A-Za-z0-9_-]+$`)
var topicPattern = regexp.MustCompile(`^ocid1\.onstopic\.oc1\.([a-z0-9][a-z0-9-]*)\.([A-Za-z0-9_-]*\.)?[A-Za-z0-9_-]+$`)
var subscriptionPattern = regexp.MustCompile(`^ocid1\.onssubscription\.oc1\.([a-z0-9][a-z0-9-]*)\.([A-Za-z0-9_-]*\.)?[A-Za-z0-9_-]+$`)

func (c *Client) regionalID(id string, pattern *regexp.Regexp) bool {
	if len(id) > 4096 {
		return false
	}
	parts := pattern.FindStringSubmatch(id)
	return parts != nil && (parts[1] == c.region || parts[1] == c.regionKey)
}
func (c *Client) compartment(id string) bool {
	return len(id) <= 4096 && (id == c.tenancy || compartmentPattern.MatchString(id))
}
