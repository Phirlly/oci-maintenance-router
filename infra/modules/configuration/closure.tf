locals {
  closure_enabled = local.s.closure_confirm && local.s.mode == "PAUSED"
  closure_intent  = try(jsondecode(base64decode(local.s.closure_review_code)), null)
  closure_valid = try(
    length(local.s.closure_review_code) <= 6000 &&
    local.s.closure_review_code == base64encode(jsonencode(local.closure_intent)) &&
    keys(local.closure_intent) == ["kind", "noticeId", "reviewFingerprint", "routingGeneration", "targetId", "version"] &&
    local.closure_intent.version == 1 &&
    alltrue([for name in ["kind", "noticeId", "reviewFingerprint", "routingGeneration", "targetId"] : local.closure_intent[name] == tostring(local.closure_intent[name])]) &&
    contains(["CLOSE_BATCH", "CLOSE_REVISION"], local.closure_intent.kind) &&
    length(local.closure_intent.noticeId) <= 4096 &&
    can(regex("^ocid1\\.announcement\\.oc1\\.\\.([A-Za-z0-9_-]*\\.)?[A-Za-z0-9_-]+$", local.closure_intent.noticeId)) &&
    alltrue([for name in ["reviewFingerprint", "routingGeneration", "targetId"] : can(regex("^[a-f0-9]{64}$", local.closure_intent[name]))]),
    false
  )
  closure_request_id = local.s.closure_request_number == null ? "" : "c1-${sha256(jsonencode(["closure-request-v1", var.installation_id, local.s.closure_request_number]))}"
  closure_request = try({
    requestId         = local.closure_request_id
    noticeId          = local.closure_intent.noticeId
    kind              = local.closure_intent.kind
    targetId          = local.closure_intent.targetId
    reviewFingerprint = local.closure_intent.reviewFingerprint
    routingGeneration = local.closure_intent.routingGeneration
  }, null)
}

output "closure_request_id" {
  description = "Configured per-notice closure identity, empty when closure is not requested."
  value       = local.closure_enabled ? local.closure_request_id : ""
}
