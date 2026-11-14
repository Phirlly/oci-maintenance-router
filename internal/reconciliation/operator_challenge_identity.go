package reconciliation

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"regexp"
	"unicode"
	"unicode/utf8"
)

var operatorLocalID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)
var operatorTopicID = regexp.MustCompile(`^ocid1\.onstopic\.oc1\.[a-z0-9][a-z0-9-]*\.([A-Za-z0-9_-]*\.)?[A-Za-z0-9_-]+$`)
var operatorSubscriptionID = regexp.MustCompile(`^ocid1\.onssubscription\.oc1\.[a-z0-9][a-z0-9-]*\.([A-Za-z0-9_-]*\.)?[A-Za-z0-9_-]+$`)

// NewOperatorIntent hashes the exact target; it neither validates email delivery
// nor proves the subscription belongs to this topic, region or tenancy.
func NewOperatorIntent(environment, installation, request string, channel OperatorChannel) (OperatorIntent, error) {
	if channel.Protocol != "EMAIL" || !operatorOpaqueText(channel.Endpoint, 2048) || utf8.RuneCountInString(channel.Endpoint) > 512 {
		return OperatorIntent{}, &OperatorChallengeError{Code: "invalid_intent"}
	}
	target, _ := json.Marshal([]string{"operator-target-v1", channel.Protocol, channel.Endpoint})
	i := OperatorIntent{EnvironmentID: environment, InstallationID: installation, RequestID: request, TopicID: channel.TopicID, SubscriptionID: channel.SubscriptionID, Protocol: channel.Protocol, EndpointDigest: operatorDigest(string(target))}
	if !validOperatorIntent(i) {
		return OperatorIntent{}, &OperatorChallengeError{Code: "invalid_intent"}
	}
	return i, nil
}
func validOperatorIntent(i OperatorIntent) bool {
	return leaseDigest.MatchString(i.EnvironmentID) && operatorLocalID.MatchString(i.InstallationID) && operatorLocalID.MatchString(i.RequestID) && len(i.TopicID) <= 4096 && operatorTopicID.MatchString(i.TopicID) && len(i.SubscriptionID) <= 4096 && operatorSubscriptionID.MatchString(i.SubscriptionID) && i.Protocol == "EMAIL" && leaseDigest.MatchString(i.EndpointDigest)
}
func operatorOpaqueText(value string, limit int) bool {
	if value == "" || len(value) > limit || !utf8.ValidString(value) {
		return false
	}
	for _, r := range value {
		if unicode.IsControl(r) || unicode.IsSpace(r) {
			return false
		}
	}
	return true
}
func operatorIntentDigest(i OperatorIntent) string {
	body, _ := json.Marshal([]string{"operator-challenge-intent-v1", i.EnvironmentID, i.InstallationID, i.RequestID, i.TopicID, i.SubscriptionID, i.Protocol, i.EndpointDigest})
	return operatorDigest(string(body))
}
func operatorDigest(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

// This code correlates a later operator attestation. It is not secret,
// authentication, or independent proof that a person received the message.
func operatorReceiptCode(intentDigest string) string {
	body, _ := json.Marshal([]string{"operator-challenge-receipt-v1", intentDigest})
	sum := sha256.Sum256(body)
	return "OMR-OP-" + hex.EncodeToString(sum[:16])
}
