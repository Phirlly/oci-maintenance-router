package configuration

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"slices"
	"strings"
	"unicode/utf8"
)

// Routing meaning is unchanged across outer configuration schemas1,2 and3.
// Preserve this projection's original bytes for saved delivery compatibility.
const routingProjectionVersion = 1

// EnvironmentID hashes the exact deployment identity and trusted ledger location.
// Use identities only on configurations returned by Decode; revalidate edits.
func (c Config) EnvironmentID(location LedgerLocation) (string, error) {
	if location.Namespace == "" || len(location.Namespace) > 1024 || !utf8.ValidString(location.Namespace) {
		return "", &Error{"ledger.namespace", "invalid"}
	}
	if !bucketPattern.MatchString(location.Bucket) {
		return "", &Error{"ledger.bucket", "invalid"}
	}
	data, err := json.Marshal([]string{c.TenancyID, c.DeploymentRegion, location.Namespace, location.Bucket})
	if err != nil {
		return "", err
	}
	return digest(data), nil
}

// RoutingGeneration identifies routing meaning, not activation authorization.
// Operational controls and presentation names are intentionally excluded.
func (c Config) RoutingGeneration() (string, error) {
	data, err := c.canonicalRouting()
	if err != nil {
		return "", err
	}
	return digest(data), nil
}

func (c Config) canonicalRouting() ([]byte, error) {
	compartments := slices.Clone(c.CompartmentIDs)
	services := slices.Clone(c.MySQLServices)
	slices.Sort(compartments)
	slices.Sort(services)
	destinations := slices.Clone(c.Destinations)
	slices.SortFunc(destinations, func(a, b Destination) int { return strings.Compare(a.ID, b.ID) })
	destinationValues := make([]map[string]string, 0, len(destinations))
	for _, destination := range destinations {
		destinationValues = append(destinationValues, map[string]string{"id": destination.ID, "secretId": destination.SecretID})
	}
	routes := slices.Clone(c.Routes)
	slices.SortFunc(routes, func(a, b Route) int { return strings.Compare(a.OwnerValue, b.OwnerValue) })
	routeValues := make([]map[string]string, 0, len(routes))
	for _, route := range routes {
		routeValues = append(routeValues, map[string]string{"ownerValue": route.OwnerValue, "destinationId": route.DestinationID})
	}
	owner := map[string]string{"kind": c.Ownership.Kind, "key": c.Ownership.Key}
	if c.Ownership.Kind == "defined" {
		owner["namespace"] = c.Ownership.Namespace
	}
	return json.Marshal(map[string]any{
		"schemaVersion": routingProjectionVersion, "tenancyId": c.TenancyID,
		"deploymentRegion": c.DeploymentRegion, "regionMap": c.RegionMap,
		"compartmentIds": compartments, "mysqlServices": services, "ownership": owner,
		"destinations": destinationValues, "routes": routeValues, "pagerDutyRegion": c.PagerDutyRegion,
	})
}

func digest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
