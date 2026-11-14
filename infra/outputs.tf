output "installation_status" {
  description = "Historical deployment check; not current health, human receipt or maintenance-delivery readiness."
  value       = try(jsondecode(oci_functions_invoke_function.deployment_check.content).status, "not-ready")
}
output "sending_enabled" {
  description = "Maintenance delivery remains disabled. TEST can send one explicitly requested nonproduction alert."
  value       = false
}
output "function_id" {
  description = "Function preparing history or executing the selected nonproduction test."
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
  value       = var.mode
}
output "test_submission_status" {
  description = "Saved test outcome. API_ACCEPTED confirms PagerDuty API acceptance, not human receipt."
  value       = try(jsondecode(oci_functions_invoke_function.deployment_check.content).diagnostic.state, "not-requested")
}
