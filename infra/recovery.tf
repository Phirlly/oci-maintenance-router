locals {
  # A fresh invocation verifies this Apply; the recorded action identity is stable.
  recovery_attempt_id = uuid()
}

resource "oci_functions_invoke_function" "recovery" {
  count                 = var.recovery_confirm ? 1 : 0
  function_id           = oci_functions_function.router.id
  fn_invoke_type        = "Sync"
  is_dry_run            = false
  base64_encode_content = false
  invoke_function_body = jsonencode({
    operation           = "recover-work"
    checkAttemptId      = local.recovery_attempt_id
    configurationSha256 = sha256(module.configuration.configuration_json)
    releaseSha256       = local.release_sha256
    recoveryRequestId   = module.configuration.recovery_request_id
  })
  depends_on = [oci_functions_invoke_function.deployment_check]
  lifecycle {
    postcondition {
      condition = try(
        var.recovery_confirm && local.mode == "LIVE" &&
        jsondecode(self.content).responseVersion == 2 &&
        jsondecode(self.content).status == "recovery-verified" &&
        jsondecode(self.content).operation == "recover-work" &&
        jsondecode(self.content).mode == "LIVE" &&
        jsondecode(self.content).checkAttemptId == local.recovery_attempt_id &&
        jsondecode(self.content).configurationSha256 == sha256(module.configuration.configuration_json) &&
        jsondecode(self.content).releaseSha256 == local.release_sha256 &&
        can(regex("^[a-f0-9]{64}$", jsondecode(self.content).routingGeneration)) &&
        jsondecode(self.content).baselinePrepared == false &&
        jsondecode(self.content).baselineSelected == false &&
        jsondecode(self.content).scanCompleted == true &&
        try(jsondecode(self.content).code, "") == "" &&
        try(jsondecode(self.content).cleanupCode, "") == "" &&
        alltrue([for name in ["diagnostic", "operator", "review", "savedNotices", "closureReview", "recoveryReview"] : try(jsondecode(self.content)[name], null) == null]) &&
        alltrue([for name in ["prepared", "accepted", "deferred", "suppressed", "issues", "attempts"] : jsondecode(self.content).processing[name] == tonumber(jsondecode(self.content).processing[name]) && jsondecode(self.content).processing[name] >= 0 && floor(jsondecode(self.content).processing[name]) == jsondecode(self.content).processing[name]]) &&
        alltrue([for name in ["issues", "reviewed", "remaining", "deferred"] : jsondecode(self.content).reporting[name] == tonumber(jsondecode(self.content).reporting[name]) && jsondecode(self.content).reporting[name] >= 0 && floor(jsondecode(self.content).reporting[name]) == jsondecode(self.content).reporting[name]]) &&
        alltrue([for name in ["known", "pending", "dispatched", "accepted"] : jsondecode(self.content).reporting[name] == tobool(jsondecode(self.content).reporting[name])]) &&
        try(jsondecode(self.content).reporting.code, "") == "" &&
        jsondecode(self.content).telemetry.accepted == true &&
        try(jsondecode(self.content).telemetry.code, "") == "" &&
        jsondecode(self.content).recovery.requestId == module.configuration.recovery_request_id &&
        contains(["RETRY_BATCH", "APPROVE_REVISION"], module.configuration.recovery_kind) &&
        jsondecode(self.content).recovery.status == (module.configuration.recovery_kind == "RETRY_BATCH" ? "API_ACCEPTED" : "APPROVED") &&
        try(jsondecode(self.content).recovery.reason, "") == "" &&
        try(jsondecode(self.content).recovery.invalidationReason, "") == "",
        false
      )
      error_message = "Recovery was not verified. Inspect the safe result and saved history. Retry uncertain outcomes with the same code and number. For a rejected request, review again and use a new number. Clear confirmation and Apply to remove intent; already approved batches remain authorized."
    }
  }
}

locals {
  recovery_result = try(jsondecode(oci_functions_invoke_function.recovery[0].content), null)
  # Retained partial results never establish successful recovery for this Apply.
  recovery_verified = try(
    var.recovery_confirm && local.mode == "LIVE" &&
    local.recovery_result.responseVersion == 2 &&
    local.recovery_result.status == "recovery-verified" &&
    local.recovery_result.operation == "recover-work" &&
    local.recovery_result.mode == "LIVE" &&
    local.recovery_result.checkAttemptId == local.recovery_attempt_id &&
    local.recovery_result.configurationSha256 == sha256(module.configuration.configuration_json) &&
    local.recovery_result.releaseSha256 == local.release_sha256 &&
    can(regex("^[a-f0-9]{64}$", local.recovery_result.routingGeneration)) &&
    local.recovery_result.baselinePrepared == false &&
    local.recovery_result.baselineSelected == false &&
    local.recovery_result.scanCompleted == true &&
    try(local.recovery_result.code, "") == "" &&
    try(local.recovery_result.cleanupCode, "") == "" &&
    alltrue([for name in ["diagnostic", "operator", "review", "savedNotices", "closureReview", "recoveryReview"] : try(local.recovery_result[name], null) == null]) &&
    alltrue([for name in ["prepared", "accepted", "deferred", "suppressed", "issues", "attempts"] : local.recovery_result.processing[name] == tonumber(local.recovery_result.processing[name]) && local.recovery_result.processing[name] >= 0 && floor(local.recovery_result.processing[name]) == local.recovery_result.processing[name]]) &&
    alltrue([for name in ["issues", "reviewed", "remaining", "deferred"] : local.recovery_result.reporting[name] == tonumber(local.recovery_result.reporting[name]) && local.recovery_result.reporting[name] >= 0 && floor(local.recovery_result.reporting[name]) == local.recovery_result.reporting[name]]) &&
    alltrue([for name in ["known", "pending", "dispatched", "accepted"] : local.recovery_result.reporting[name] == tobool(local.recovery_result.reporting[name])]) &&
    try(local.recovery_result.reporting.code, "") == "" &&
    local.recovery_result.telemetry.accepted == true &&
    try(local.recovery_result.telemetry.code, "") == "" &&
    local.recovery_result.recovery.requestId == module.configuration.recovery_request_id &&
    contains(["RETRY_BATCH", "APPROVE_REVISION"], module.configuration.recovery_kind) &&
    local.recovery_result.recovery.status == (module.configuration.recovery_kind == "RETRY_BATCH" ? "API_ACCEPTED" : "APPROVED") &&
    try(local.recovery_result.recovery.reason, "") == "" &&
    try(local.recovery_result.recovery.invalidationReason, "") == "",
    false
  )
}

output "recovery_verified" {
  description = "Exact recovery verified by this Apply. Retry confirms API acceptance, not human receipt; revision approval confirms authorization, not delivery."
  value       = local.recovery_verified
}
output "recovery_result" {
  description = "Safe result from this recovery invocation, including partial outcomes. Inspect retained history after uncertainty."
  value       = try(oci_functions_invoke_function.recovery[0].content, "not-requested")
}
