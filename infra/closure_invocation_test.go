package infra_test

import (
	"encoding/json"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

func closureResult() map[string]any {
	return map[string]any{"responseVersion": 2, "status": "closure-completed", "operation": "close-work", "mode": "PAUSED",
		"checkAttemptId": "12345678-1234-1234-1234-123456789abc", "configurationSha256": strings.Repeat("a", 64), "releaseSha256": strings.Repeat("b", 64),
		"routingGeneration": strings.Repeat("c", 64), "baselinePrepared": false, "baselineSelected": false, "scanCompleted": false,
		"processing": map[string]any{"prepared": 0, "accepted": 0, "deferred": 0, "suppressed": 0, "issues": 0, "attempts": 0},
		"recovery":   map[string]any{"requestId": "c1-request", "status": "CLOSED", "ambiguous": false}}
}

func TestClosureInvocationRejectsStalePartialAndOtherWork(t *testing.T) {
	source, err := os.ReadFile("closure.tf")
	if err != nil {
		t.Fatal(err)
	}
	match := regexp.MustCompile(`(?s)postcondition\s*\{\s*condition\s*=\s*(.*?)\s*error_message\s*=`).FindSubmatch(source)
	if len(match) != 2 {
		t.Fatal("missing closure predicate")
	}
	outputMatch := regexp.MustCompile(`(?s)closure_verified\s*=\s*(.*?)\n}`).FindSubmatch(source)
	if len(outputMatch) != 2 {
		t.Fatal("missing closure output predicate")
	}
	for _, kind := range []string{"valid", "version", "operation", "mode", "status", "attempt", "configuration", "release", "generation", "prepared", "selected", "scan", "code", "cleanup", "diagnostic", "operator", "reporting", "review", "savedNotices", "closureReview", "work", "missing-count", "request", "rejected", "ambiguous", "reason", "invalidation", "missing", "malformed", "typed"} {
		t.Run(kind, func(t *testing.T) {
			v := closureResult()
			r := v["recovery"].(map[string]any)
			p := v["processing"].(map[string]any)
			switch kind {
			case "version":
				v["responseVersion"] = "2"
			case "operation":
				v["operation"] = "reconcile"
			case "mode":
				v["mode"] = "LIVE"
			case "status":
				v["status"] = "not-ready"
			case "attempt":
				v["checkAttemptId"] = "previous"
			case "configuration":
				v["configurationSha256"] = strings.Repeat("d", 64)
			case "release":
				v["releaseSha256"] = strings.Repeat("d", 64)
			case "generation":
				v["routingGeneration"] = "bad"
			case "prepared":
				v["baselinePrepared"] = true
			case "selected":
				v["baselineSelected"] = true
			case "scan":
				v["scanCompleted"] = true
			case "code":
				v["code"] = "failed"
			case "cleanup":
				v["cleanupCode"] = "failed"
			case "diagnostic", "operator", "reporting", "review", "savedNotices", "closureReview":
				v[kind] = map[string]any{}
			case "work":
				p["attempts"] = 1
			case "missing-count":
				delete(p, "issues")
			case "request":
				r["requestId"] = "previous"
			case "rejected":
				r["status"] = "REJECTED"
			case "ambiguous":
				r["ambiguous"] = true
			case "reason":
				r["reason"] = "stale_review"
			case "invalidation":
				r["invalidationReason"] = "changed"
			case "missing":
				delete(v, "recovery")
			case "typed":
				p["attempts"] = "0"
			}
			body, _ := json.Marshal(v)
			if kind == "malformed" {
				body = []byte("bad")
			}
			for _, predicate := range []string{string(match[1]), strings.ReplaceAll(string(outputMatch[1]), "local.closure_result", "jsondecode(self.content)")} {
				expression := strings.NewReplacer("self.content", strconv.Quote(string(body)), "local.closure_attempt_id", `"12345678-1234-1234-1234-123456789abc"`,
					"sha256(module.configuration.configuration_json)", strconv.Quote(strings.Repeat("a", 64)), "local.release_sha256", strconv.Quote(strings.Repeat("b", 64)),
					"module.configuration.closure_request_id", `"c1-request"`, "var.closure_confirm", "true", "var.mode", `"PAUSED"`, "\n", " ").Replace(predicate)
				got, err := terraform(t, t.TempDir(), expression+"\n", "console", "-no-color")
				if err != nil || strings.TrimSpace(string(got)) != strconv.FormatBool(kind == "valid") {
					t.Fatalf("predicate %s: %s (%v)", kind, got, err)
				}
			}
		})
	}
}
