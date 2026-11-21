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
  description    = "Selected-service wake-ups; current resource ownership and scope are resolved by the router."
  ons_topic_id   = oci_ons_notification_topic.intake.id
  filter_groups {
    dynamic "filters" {
      # API filters use current names; runtime matching also accepts historical names.
      for_each = toset(local.subscription_services)
      content {
        type  = "SERVICE"
        value = filters.value
      }
    }
  }
  lifecycle {
    precondition {
      condition     = local.service_catalog_valid
      error_message = "Release catalogs must contain MySQL and Compute with nonempty, unique labels; subscription labels must belong to the matching runtime service catalog."
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
locals {
  subscription_services = sort(distinct(flatten([for kind in local.resource_type_ids : try(local.release.subscriptionServices[kind], [])])))
  service_catalog_valid = try(
    toset(keys(local.release.resourceServices)) == toset(["instance", "mysqldbsystem"]) &&
    toset(keys(local.release.subscriptionServices)) == toset(["instance", "mysqldbsystem"]) &&
    alltrue([for kind, services in local.release.resourceServices :
      length(services) > 0 && length(distinct(services)) == length(services) && alltrue([for service in services : trimspace(service) != ""]) &&
      length(local.release.subscriptionServices[kind]) > 0 && length(distinct(local.release.subscriptionServices[kind])) == length(local.release.subscriptionServices[kind]) &&
      length(setsubtract(toset(local.release.subscriptionServices[kind]), toset(services))) == 0
    ]), false
  )
}
