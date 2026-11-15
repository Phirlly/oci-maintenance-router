locals {
  recovery_enabled = local.s.recovery_confirm && local.s.mode == "LIVE"
  recovery_intent  = try(jsondecode(base64decode(local.s.recovery_review_code)), null)
  recovery_valid = try(
    length(local.s.recovery_review_code) <= 6000 &&
    local.s.recovery_review_code == base64encode(jsonencode(local.recovery_intent)) &&
    keys(local.recovery_intent) == ["componentFingerprint", "kind", "noticeId", "reviewFingerprint", "routingGeneration", "targetId", "version"] &&
    local.recovery_intent.version == 1 &&
    alltrue([for name in ["componentFingerprint", "kind", "noticeId", "reviewFingerprint", "routingGeneration", "targetId"] : local.recovery_intent[name] == tostring(local.recovery_intent[name])]) &&
    contains(["RETRY_BATCH", "APPROVE_REVISION"], local.recovery_intent.kind) &&
    length(local.recovery_intent.noticeId) <= 4096 &&
    can(regex("^ocid1\\.announcement\\.oc1\\.\\.([A-Za-z0-9_-]*\\.)?[A-Za-z0-9_-]+$", local.recovery_intent.noticeId)) &&
    alltrue([for name in ["componentFingerprint", "reviewFingerprint", "routingGeneration", "targetId"] : can(regex("^[a-f0-9]{64}$", local.recovery_intent[name]))]),
    false
  )
  recovery_request_id = local.s.recovery_request_number == null ? "" : "r1-${sha256(jsonencode(["sending-recovery-v1", var.installation_id, local.s.recovery_request_number]))}"
  recovery_request = try({
    requestId            = local.recovery_request_id
    noticeId             = local.recovery_intent.noticeId
    kind                 = local.recovery_intent.kind
    targetId             = local.recovery_intent.targetId
    reviewFingerprint    = local.recovery_intent.reviewFingerprint
    routingGeneration    = local.recovery_intent.routingGeneration
    componentFingerprint = local.recovery_intent.componentFingerprint
  }, null)
}

output "recovery_request_id" {
  description = "Configured per-notice sending recovery identity, empty when not requested."
  value       = local.recovery_enabled ? local.recovery_request_id : ""
}
output "recovery_kind" {
  description = "Reviewed recovery action, empty when not requested."
  value       = local.recovery_enabled ? try(local.recovery_intent.kind, "") : ""
}
