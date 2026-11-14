package configuration

import (
	"strings"
	"testing"
)

func TestOperatorRequestAndSubscriptionBounds(t *testing.T) {
	for _, id := range []string{"", "../id", "with space", "é", strings.Repeat("a", 129)} {
		v := operatorSettings(t)
		v["operatorTest"].(map[string]any)["requestId"] = id
		wantError(t, encoded(t, v), "operatorTest.requestId", "invalid")
	}
	for _, id := range []string{
		"", "ocid1.onssubscription.oc2.iad.test",
		"ocid1.onssubscription.oc1.phx.test", "ocid1.onssubscription.oc1..test",
		"ocid1.onssubscription.oc1.iad.", "ocid1.onstopic.oc1.iad.test",
	} {
		v := operatorSettings(t)
		v["operatorTest"].(map[string]any)["subscriptionId"] = id
		wantError(t, encoded(t, v), "operatorTest.subscriptionId", "invalid")
	}
	for _, id := range []string{
		operatorSubscription, "ocid1.onssubscription.oc1.us-ashburn-1.test",
		"ocid1.onssubscription.oc1.iad..test", "ocid1.onssubscription.oc1.iad.future.test",
	} {
		v := operatorSettings(t)
		o := v["operatorTest"].(map[string]any)
		o["requestId"], o["subscriptionId"] = strings.Repeat("A", 128), id
		if decoded(t, v).OperatorTest.SubscriptionID != id {
			t.Fatal("identifier spelling changed")
		}
	}
	for _, kind := range []string{"onssubscription", "onstopic"} {
		v := operatorSettings(t)
		prefix := "ocid1." + kind + ".oc1.iad."
		id := prefix + strings.Repeat("a", 4096-len(prefix))
		field := "operatorTest.subscriptionId"
		set := func(id string) { v["operatorTest"].(map[string]any)["subscriptionId"] = id }
		if kind == "onstopic" {
			field = "operatorTopicId"
			set = func(id string) { v["operatorTopicId"] = id }
		}
		set(id)
		decoded(t, v)
		set(id + "a")
		wantError(t, encoded(t, v), field, "invalid")
	}
}

func TestOperatorEndpointAndReceiptBounds(t *testing.T) {
	for _, endpoint := range []string{"Team@example.invalid", strings.Repeat("a", 512), strings.Repeat("😀", 512)} {
		v := operatorSettings(t)
		v["operatorTest"].(map[string]any)["endpoint"] = endpoint
		if decoded(t, v).OperatorTest.Endpoint != endpoint {
			t.Fatal("contact changed")
		}
	}
	for _, endpoint := range []string{
		"", " a", "a b", "a\n", "a\t", "a\u0000", "a\u0085", "a\u00a0", "a\u2003",
		strings.Repeat("a", 513), strings.Repeat("😀", 513),
	} {
		v := operatorSettings(t)
		v["operatorTest"].(map[string]any)["endpoint"] = endpoint
		wantError(t, encoded(t, v), "operatorTest.endpoint", "invalid")
	}
	for _, code := range []string{
		receiptCode, "OMR-OP-", strings.ToLower(operatorCode), strings.ToUpper(operatorCode),
		operatorCode + "0", operatorCode[:len(operatorCode)-1], " " + operatorCode,
		operatorCode + "\n", "OMR-OP-" + strings.Repeat("g", 32),
	} {
		v := operatorSettings(t)
		v["operatorTest"].(map[string]any)["receiptCode"] = code
		wantError(t, encoded(t, v), "operatorTest.receiptCode", "invalid")
	}
}
