output "installation_status" {
  description = "Historical deployment check; not current health, human receipt or maintenance-delivery readiness."
  value       = try(jsondecode(oci_functions_invoke_function.deployment_check.content).status, "not-ready")
}
output "sending_enabled" {
  description = "LIVE activation verified by this Apply. Not current health, automatic invocation or observed maintenance delivery."
  value       = local.live_check_verified
}
output "function_id" {
  description = "Function for maintenance preparation, explicit tests and guarded LIVE processing."
  value       = oci_functions_function.router.id
}
output "state_bucket" {
  description = "Private versioned storage protected from normal Terraform destruction."
  value       = oci_objectstorage_bucket.state.name
}
output "check_result" {
  description = "Safe result for this Apply; contains no secret, baseline contents or human receipt proof."
  value       = oci_functions_invoke_function.deployment_check.content
}
output "configured_mode" {
  description = "Mode used for this deployment check."
  value       = local.mode
}
output "routing_state" {
  description = "Configured maintenance routing. See sending_enabled for activation verified by the last Apply."
  value       = var.routing_state
}
output "test_submission_status" {
  description = "Saved test outcome. API_ACCEPTED confirms PagerDuty API acceptance, not human receipt."
  value       = try(jsondecode(oci_functions_invoke_function.deployment_check.content).diagnostic.state, "not-requested")
}

locals {
  # Never promote a partial proof preserved in a failed or stale Apply result.
  receipt_check_verified = try(
    local.receipt_confirmation && local.mode == "TEST" &&
    jsondecode(oci_functions_invoke_function.deployment_check.content).responseVersion == 2 &&
    jsondecode(oci_functions_invoke_function.deployment_check.content).status == "operational-verified" &&
    jsondecode(oci_functions_invoke_function.deployment_check.content).operation == "deployment-check" &&
    jsondecode(oci_functions_invoke_function.deployment_check.content).mode == local.mode &&
    jsondecode(oci_functions_invoke_function.deployment_check.content).checkAttemptId == local.check_attempt_id &&
    jsondecode(oci_functions_invoke_function.deployment_check.content).configurationSha256 == sha256(module.configuration.configuration_json) &&
    jsondecode(oci_functions_invoke_function.deployment_check.content).releaseSha256 == local.release_sha256 &&
    can(regex("^[a-f0-9]{64}$", jsondecode(oci_functions_invoke_function.deployment_check.content).routingGeneration)) &&
    jsondecode(oci_functions_invoke_function.deployment_check.content).baselinePrepared == false &&
    jsondecode(oci_functions_invoke_function.deployment_check.content).baselineSelected == false &&
    jsondecode(oci_functions_invoke_function.deployment_check.content).scanCompleted == false &&
    try(jsondecode(oci_functions_invoke_function.deployment_check.content).code, "") == "" &&
    try(jsondecode(oci_functions_invoke_function.deployment_check.content).cleanupCode, "") == "" &&
    try(jsondecode(oci_functions_invoke_function.deployment_check.content).processing, null) == null &&
    try(jsondecode(oci_functions_invoke_function.deployment_check.content).reporting, null) == null &&
    try(jsondecode(oci_functions_invoke_function.deployment_check.content).telemetry, null) == null &&
    try(jsondecode(oci_functions_invoke_function.deployment_check.content).recoveryReview, null) == null &&
    try(jsondecode(oci_functions_invoke_function.deployment_check.content).recovery, null) == null &&
    (local.operator_test_enabled || try(jsondecode(oci_functions_invoke_function.deployment_check.content).operator, null) == null) &&
    alltrue([for channel in concat(["diagnostic"], local.operator_test_enabled ? ["operator"] : []) :
      jsondecode(oci_functions_invoke_function.deployment_check.content)[channel].state == "API_ACCEPTED" &&
      jsondecode(oci_functions_invoke_function.deployment_check.content)[channel].receiptVerified == true &&
      jsondecode(oci_functions_invoke_function.deployment_check.content)[channel].dispatched == false &&
      !contains(keys(jsondecode(oci_functions_invoke_function.deployment_check.content)[channel]), "observedStatus") &&
      !contains(keys(jsondecode(oci_functions_invoke_function.deployment_check.content)[channel]), "code")
    ]),
    false
  )
}
output "test_receipt_confirmed" {
  description = "Receipt code verified by the last completed TEST confirmation check. Not current health or maintenance activation."
  value       = local.receipt_check_verified
}
output "operator_receipt_confirmed" {
  description = "Operator email code verified by the last completed TEST confirmation check. Not current health or maintenance activation."
  value       = local.operator_test_enabled && local.receipt_check_verified
}
