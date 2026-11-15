locals {
  # Correlation changes each Apply; the configured request identity does not.
  closure_attempt_id = uuid()
}

resource "oci_functions_invoke_function" "closure" {
  count                 = var.closure_confirm ? 1 : 0
  function_id           = oci_functions_function.router.id
  fn_invoke_type        = "Sync"
  is_dry_run            = false
  base64_encode_content = false
  invoke_function_body = jsonencode({
    operation           = "close-work"
    checkAttemptId      = local.closure_attempt_id
    configurationSha256 = sha256(module.configuration.configuration_json)
    releaseSha256       = local.release_sha256
    recoveryRequestId   = module.configuration.closure_request_id
  })
  depends_on = [oci_functions_invoke_function.deployment_check]
  lifecycle {
    postcondition {
      condition = try(
        jsondecode(self.content).responseVersion == 2 &&
        jsondecode(self.content).status == "closure-completed" &&
        jsondecode(self.content).operation == "close-work" &&
        jsondecode(self.content).mode == "PAUSED" &&
        jsondecode(self.content).checkAttemptId == local.closure_attempt_id &&
        jsondecode(self.content).configurationSha256 == sha256(module.configuration.configuration_json) &&
        jsondecode(self.content).releaseSha256 == local.release_sha256 &&
        can(regex("^[a-f0-9]{64}$", jsondecode(self.content).routingGeneration)) &&
        jsondecode(self.content).baselinePrepared == false &&
        jsondecode(self.content).baselineSelected == false &&
        jsondecode(self.content).scanCompleted == false &&
        try(jsondecode(self.content).code, "") == "" &&
        try(jsondecode(self.content).cleanupCode, "") == "" &&
        alltrue([for name in ["diagnostic", "operator", "reporting", "telemetry", "review", "savedNotices", "closureReview", "recoveryReview"] : try(jsondecode(self.content)[name], null) == null]) &&
        alltrue([for name in ["prepared", "accepted", "deferred", "suppressed", "issues", "attempts"] : jsondecode(self.content).processing[name] == 0]) &&
        jsondecode(self.content).recovery.requestId == module.configuration.closure_request_id &&
        jsondecode(self.content).recovery.status == "CLOSED" &&
        jsondecode(self.content).recovery.ambiguous == false &&
        try(jsondecode(self.content).recovery.reason, "") == "" &&
        try(jsondecode(self.content).recovery.invalidationReason, "") == "",
        false
      )
      error_message = "Closure did not complete. Inspect the safe result or saved history. Retry uncertain outcomes with the same code and number. For a rejected request, review again and use a new number. Clear confirmation and Apply to remove intent. History is preserved; nothing is sent."
    }
  }
}

locals {
  closure_result = try(jsondecode(oci_functions_invoke_function.closure[0].content), null)
  # Failed/stale Apply state must not become a closure-completed output.
  closure_verified = try(
    var.closure_confirm && var.mode == "PAUSED" &&
    local.closure_result.responseVersion == 2 &&
    local.closure_result.status == "closure-completed" &&
    local.closure_result.operation == "close-work" &&
    local.closure_result.mode == "PAUSED" &&
    local.closure_result.checkAttemptId == local.closure_attempt_id &&
    local.closure_result.configurationSha256 == sha256(module.configuration.configuration_json) &&
    local.closure_result.releaseSha256 == local.release_sha256 &&
    can(regex("^[a-f0-9]{64}$", local.closure_result.routingGeneration)) &&
    local.closure_result.baselinePrepared == false &&
    local.closure_result.baselineSelected == false &&
    local.closure_result.scanCompleted == false &&
    try(local.closure_result.code, "") == "" &&
    try(local.closure_result.cleanupCode, "") == "" &&
    alltrue([for name in ["diagnostic", "operator", "reporting", "telemetry", "review", "savedNotices", "closureReview", "recoveryReview"] : try(local.closure_result[name], null) == null]) &&
    alltrue([for name in ["prepared", "accepted", "deferred", "suppressed", "issues", "attempts"] : local.closure_result.processing[name] == 0]) &&
    local.closure_result.recovery.requestId == module.configuration.closure_request_id &&
    local.closure_result.recovery.status == "CLOSED" &&
    local.closure_result.recovery.ambiguous == false &&
    try(local.closure_result.recovery.reason, "") == "" &&
    try(local.closure_result.recovery.invalidationReason, "") == "",
    false
  )
}

output "closure_completed" {
  description = "Exact requested closure verified by this Apply; not delivery acceptance or absence of earlier send uncertainty."
  value       = local.closure_verified
}
output "closure_result" {
  description = "Safe saved-action result from this closure invocation, including rejection. Inspect retained history after an uncertain outcome."
  value       = try(oci_functions_invoke_function.closure[0].content, "not-requested")
}
