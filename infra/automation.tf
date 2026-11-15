# Stable subscriptions may deliver after a mode change. The runtime treats every
# automatic request outside LIVE as idle; topic messages never become commands.
resource "oci_ons_subscription" "intake" {
  compartment_id = var.compartment_ocid
  topic_id       = oci_ons_notification_topic.intake.id
  protocol       = "ORACLE_FUNCTIONS"
  endpoint       = oci_functions_function.router.id
  depends_on     = [oci_functions_invoke_function.deployment_check]
}
resource "oci_announcements_service_announcement_subscription" "mysql" {
  compartment_id = var.compartment_ocid
  display_name   = "${local.name}-mysql"
  description    = "MySQL service wake-ups; current ownership and scope are resolved by the router."
  ons_topic_id   = oci_ons_notification_topic.intake.id
  filter_groups {
    dynamic "filters" {
      for_each = toset(local.catalog.mysqlServices)
      content {
        type  = "SERVICE"
        value = filters.value
      }
    }
  }
  depends_on = [oci_ons_subscription.intake]
}
resource "oci_resource_scheduler_schedule" "reconcile" {
  compartment_id     = var.compartment_ocid
  display_name       = "${local.name}-reconcile"
  description        = "Hourly API reconciliation; invocation acceptance is not completed processing."
  action             = "START_RESOURCE"
  recurrence_type    = "CRON"
  recurrence_details = "0 * * * *"
  local_time_zone    = "UTC"
  # Runtime mode controls processing; keep the trigger and its IAM identity stable.
  state = "ACTIVE"
  resources {
    id = oci_functions_function.router.id
    parameters {
      parameter_type = "BODY"
      # The provider decodes one JSON layer; OCI stores the body as JSON text.
      value = [jsonencode(jsonencode({ operation = "wake-up" }))]
    }
  }
  # Exact schedule-ID IAM follows creation during nonsending setup.
  depends_on = [oci_functions_invoke_function.deployment_check]
}
