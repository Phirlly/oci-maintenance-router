output "installation_status" {
  description = "Historical PREVIEW preparation result; not current health or delivery readiness."
  value       = try(jsondecode(oci_functions_invoke_function.deployment_check.content).status, "not-ready")
}
output "sending_enabled" {
  description = "Always false for this PREVIEW-only release."
  value       = false
}
output "function_id" {
  description = "Function preparing maintenance history without sending notifications."
  value       = oci_functions_function.router.id
}
output "state_bucket" {
  description = "Private versioned storage protected from normal Terraform destruction."
  value       = oci_objectstorage_bucket.state.name
}
output "check_result" {
  description = "Safe PREVIEW result for this Apply; contains no baseline contents or delivery receipt."
  value       = oci_functions_invoke_function.deployment_check.content
}
