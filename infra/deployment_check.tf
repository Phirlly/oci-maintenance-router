locals {
  # Unknown at plan time, new for EVERY Apply; reused within this one attempt.
  check_attempt_id = uuid()
}
resource "oci_functions_invoke_function" "deployment_check" {
  function_id           = oci_functions_function.router.id
  fn_invoke_type        = "Sync"
  is_dry_run            = false
  base64_encode_content = false
  invoke_function_body = jsonencode({
    operation           = "deployment-check"
    checkAttemptId      = local.check_attempt_id
    configurationSha256 = sha256(module.configuration.configuration_json)
    releaseSha256       = local.release_sha256
  })
  depends_on = [oci_identity_policy.router, oci_identity_policy.announcements, oci_identity_policy.diagnostic, data.oci_functions_function.archive]
  lifecycle {
    postcondition {
      condition = try(
        jsondecode(self.content).responseVersion == 2 &&
        jsondecode(self.content).status == "operational-verified" &&
        jsondecode(self.content).operation == "deployment-check" &&
        jsondecode(self.content).mode == var.mode &&
        jsondecode(self.content).checkAttemptId == local.check_attempt_id &&
        jsondecode(self.content).configurationSha256 == sha256(module.configuration.configuration_json) &&
        jsondecode(self.content).releaseSha256 == local.release_sha256 &&
        can(regex("^[a-f0-9]{64}$", jsondecode(self.content).routingGeneration)) &&
        jsondecode(self.content).baselinePrepared == (var.mode == "PREVIEW") &&
        jsondecode(self.content).baselineSelected == false &&
        jsondecode(self.content).scanCompleted == false &&
        try(jsondecode(self.content).code, "") == "" &&
        try(jsondecode(self.content).cleanupCode, "") == "" &&
        (var.mode == "TEST" ? (
          jsondecode(self.content).diagnostic.state == "API_ACCEPTED" &&
          try(jsondecode(self.content).diagnostic.code, "") == "" &&
          ((jsondecode(self.content).diagnostic.dispatched == true && try(jsondecode(self.content).diagnostic.observedStatus, 0) == 202) ||
          (jsondecode(self.content).diagnostic.dispatched == false && try(jsondecode(self.content).diagnostic.observedStatus, 0) == 0))
          ) : (
          contains(["PREVIEW", "PAUSED"], var.mode) && try(jsondecode(self.content).diagnostic, null) == null
        )) &&
        try(jsondecode(self.content).operator, null) == null &&
        try(jsondecode(self.content).processing, null) == null &&
        try(jsondecode(self.content).recovery, null) == null,
        false
      )
      error_message = "Deployment check failed. Review the safe check result and IAM/network access. Keep the test run number unchanged when retrying; identity_conflict requires an intentional new test number. Saved progress is retained. API acceptance does not confirm receipt; maintenance delivery remains disabled."
    }
  }
}
