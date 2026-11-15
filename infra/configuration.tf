locals {
  # Retained bucket identity survives package rollback without another resource.
  installation_id      = "b1-${sha256(oci_objectstorage_bucket.state.bucket_id)}"
  receipt_confirmation = module.configuration.receipt_confirmation
}
module "configuration" {
  source                   = "./modules/configuration"
  catalog                  = local.catalog
  installation_id          = local.installation_id
  operator_subscription_id = try(oci_ons_subscription.operator[0].id, "")
  settings = {
    environment_name       = var.environment_name
    tenancy_ocid           = var.tenancy_ocid
    region                 = var.region
    mysql_regions          = var.mysql_regions
    mysql_compartments     = var.mysql_compartments
    ownership_kind         = var.ownership_kind
    ownership_namespace    = var.ownership_namespace
    ownership_key          = var.ownership_key
    destinations           = var.destinations
    routes                 = var.routes
    pagerduty_region       = var.pagerduty_region
    mode                   = var.mode
    test_secret_ocid       = var.test_secret_ocid
    test_nonproduction     = var.test_nonproduction
    test_run               = var.test_run
    test_receipt_code      = var.test_receipt_code
    operator_email         = var.operator_email
    operator_test_run      = var.operator_test_run
    operator_receipt_code  = var.operator_receipt_code
    closure_review_code    = var.closure_review_code
    closure_request_number = var.closure_request_number
    closure_confirm        = var.closure_confirm
  }
  topics = {
    intake   = oci_ons_notification_topic.intake.id
    operator = oci_ons_notification_topic.operator.id
  }
}
# Intake wakes reconciliation; operator messages never feed back into the router.
resource "oci_ons_notification_topic" "intake" {
  compartment_id = var.compartment_ocid
  name           = "${local.name}-intake"
  description    = "MySQL maintenance wake-ups; automatic processing runs only in LIVE."
}
resource "oci_ons_notification_topic" "operator" {
  compartment_id = var.compartment_ocid
  name           = "${local.name}-operator"
  description    = "Dedicated router issues, alarms and platform failures, with an explicit setup test."
}
