locals {
  operator_test_enabled = var.mode == "TEST" && var.operator_email != ""
}

# The subscription survives mode changes. OCI sends its confirmation email on
# creation/replacement, independently of the Function's test permissions.
resource "oci_ons_subscription" "operator" {
  count          = var.operator_email != "" ? 1 : 0
  compartment_id = var.compartment_ocid
  topic_id       = oci_ons_notification_topic.operator.id
  protocol       = "EMAIL"
  endpoint       = var.operator_email
}

output "operator_subscription_status" {
  description = "Historical subscription state. If PENDING, confirm OCI's email before TEST; replacement requires confirmation again."
  value       = try(oci_ons_subscription.operator[0].state, "not-configured")
}
output "operator_test_status" {
  description = "Saved operator test outcome; API_ACCEPTED confirms publication, not email receipt."
  value       = try(jsondecode(oci_functions_invoke_function.deployment_check.content).operator.state, "not-requested")
}
