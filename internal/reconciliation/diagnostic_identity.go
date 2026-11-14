package reconciliation

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"regexp"
)

var diagnosticID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)
var diagnosticSecret = regexp.MustCompile(`^ocid1\.vaultsecret\.oc1\.[a-z0-9][a-z0-9-]*\.([A-Za-z0-9_-]*\.)?[A-Za-z0-9_-]+$`)

func validDiagnosticIntent(i DiagnosticIntent) bool {
	return leaseDigest.MatchString(i.EnvironmentID) && diagnosticID.MatchString(i.InstallationID) && diagnosticID.MatchString(i.RequestID) && leaseDigest.MatchString(i.ReleaseSHA256) && leaseDigest.MatchString(i.CatalogSHA256) && (i.PagerDutyRegion == "US" || i.PagerDutyRegion == "EU") && len(i.SecretID) <= 4096 && diagnosticSecret.MatchString(i.SecretID) && i.NonProduction
}
func diagnosticIdentity(i DiagnosticIntent) (string, string) {
	intent, _ := json.Marshal([]any{"diagnostic-intent-v1", i.EnvironmentID, i.InstallationID, i.RequestID, i.ReleaseSHA256, i.CatalogSHA256, i.PagerDutyRegion, i.SecretID, true})
	dedup, _ := json.Marshal([]string{"diagnostic-dedup-v1", i.EnvironmentID, i.InstallationID, i.RequestID})
	return diagnosticDigest(string(intent)), "omr-test1:" + diagnosticDigest(string(dedup))
}
func diagnosticDigest(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

// Correlates an operator's later attestation to this exact test. It is neither
// secret nor authentication, and cannot by itself prove actual human receipt.
func diagnosticReceiptCode(intentDigest string) string {
	preimage, _ := json.Marshal([]string{"diagnostic-receipt-v1", intentDigest})
	sum := sha256.Sum256(preimage)
	return "OMR-" + hex.EncodeToString(sum[:16])
}
