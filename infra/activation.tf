locals {
  # Historical authority established by this Apply, never current health or
  # proof of delivery. A partial or stale selection must not enable the output.
  live_check_verified = try(
    var.mode == "LIVE" &&
    jsondecode(oci_functions_invoke_function.deployment_check.content).responseVersion == 2 &&
    jsondecode(oci_functions_invoke_function.deployment_check.content).status == "operational-verified" &&
    jsondecode(oci_functions_invoke_function.deployment_check.content).operation == "deployment-check" &&
    jsondecode(oci_functions_invoke_function.deployment_check.content).mode == "LIVE" &&
    jsondecode(oci_functions_invoke_function.deployment_check.content).checkAttemptId == local.check_attempt_id &&
    jsondecode(oci_functions_invoke_function.deployment_check.content).configurationSha256 == sha256(module.configuration.configuration_json) &&
    jsondecode(oci_functions_invoke_function.deployment_check.content).releaseSha256 == local.release_sha256 &&
    can(regex("^[a-f0-9]{64}$", jsondecode(oci_functions_invoke_function.deployment_check.content).routingGeneration)) &&
    jsondecode(oci_functions_invoke_function.deployment_check.content).baselineSelected == true &&
    jsondecode(oci_functions_invoke_function.deployment_check.content).baselinePrepared == false &&
    jsondecode(oci_functions_invoke_function.deployment_check.content).scanCompleted == false &&
    try(jsondecode(oci_functions_invoke_function.deployment_check.content).code, "") == "" &&
    try(jsondecode(oci_functions_invoke_function.deployment_check.content).cleanupCode, "") == "" &&
    alltrue([for key in ["diagnostic", "operator", "processing", "reporting", "telemetry", "recovery", "recoveryReview"] : try(jsondecode(oci_functions_invoke_function.deployment_check.content)[key], null) == null]),
    false
  )
}
