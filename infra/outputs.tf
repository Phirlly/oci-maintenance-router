output "installation_status" {
  description = "Historical installation verification only; this package cannot send notifications."
  value       = try(jsondecode(oci_functions_invoke_function.deployment_check.content).status, "not-ready")
}
output "sending_enabled" {
  description = "Always false for this PREVIEW-only release."
  value       = false
}
output "function_id" {
  description = "Function performing the nonsending deployment check."
  value       = oci_functions_function.router.id
}
output "state_bucket" {
  description = "Private versioned storage protected from normal Terraform destruction."
  value       = oci_objectstorage_bucket.state.name
}
output "check_result" {
  description = "Safe diagnostic result for this Apply; never a maintenance baseline or delivery receipt."
  value       = oci_functions_invoke_function.deployment_check.content
}
