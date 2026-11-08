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
  depends_on = [oci_identity_policy.router]
  lifecycle {
    postcondition {
      condition = try(
        jsondecode(self.content).status == "deployment-verified" &&
        jsondecode(self.content).sendingEnabled == false &&
        jsondecode(self.content).checkAttemptId == local.check_attempt_id &&
        jsondecode(self.content).configurationSha256 == sha256(module.configuration.configuration_json) &&
        jsondecode(self.content).releaseSha256 == local.release_sha256 &&
        can(regex("^[a-f0-9]{64}$", jsondecode(self.content).routingGeneration)),
        false
      )
      error_message = "PREVIEW installation check failed. Review the safe check result and IAM/network access, then rerun Apply after propagation. No notification delivery is enabled."
    }
  }
}
