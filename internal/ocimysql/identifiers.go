package ocimysql

import "regexp"

var regionLabel = regexp.MustCompile(`^[a-z]([a-z0-9-]*[a-z0-9])?$`)
var regionKeyLabel = regexp.MustCompile(`^[a-z0-9]+$`)
var tenancyPattern = regexp.MustCompile(`^ocid1\.tenancy\.oc1\.\.([A-Za-z0-9_-]*\.)?[A-Za-z0-9_-]+$`)
var compartmentPattern = regexp.MustCompile(`^ocid1\.compartment\.oc1\.\.([A-Za-z0-9_-]*\.)?[A-Za-z0-9_-]+$`)
var dbPattern = regexp.MustCompile(`^ocid1\.mysqldbsystem\.oc1\.([a-z0-9][a-z0-9-]*)\.([A-Za-z0-9_-]*\.)?[A-Za-z0-9_-]+$`)

func (c *Client) validDBID(id string) bool {
	if len(id) > 4096 {
		return false
	}
	parts := dbPattern.FindStringSubmatch(id)
	return parts != nil && (parts[1] == c.region || parts[1] == c.regionKey)
}
