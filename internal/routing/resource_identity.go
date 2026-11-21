package routing

import "regexp"

// The closed safe identity shapes shared by routing, dispatch and suppression.
// Unknown services/realms and malformed identifiers must never select a reader.
var resourceID = regexp.MustCompile(`^ocid1\.(mysqldbsystem|instance)\.oc1\.([a-z0-9][a-z0-9-]*)\.([A-Za-z0-9_-]*\.)?[A-Za-z0-9_-]+$`)

func resourceIdentity(id string) (kind, region string) {
	parts := resourceID.FindStringSubmatch(id)
	if parts != nil {
		return parts[1], parts[2]
	}
	return "", ""
}

// ResourceType returns only a supported safe identity's type, never source text.
// It does not establish selection, location, ownership or delivery authorization.
func ResourceType(id string) string {
	kind, _ := resourceIdentity(id)
	return kind
}
