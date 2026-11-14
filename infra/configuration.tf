locals {
  # Retained bucket identity survives package rollback without another resource.
  installation_id = "b1-${sha256(oci_objectstorage_bucket.state.bucket_id)}"
}
module "configuration" {
  source          = "./modules/configuration"
  catalog         = local.catalog
  installation_id = local.installation_id
  settings = {
    environment_name    = var.environment_name
    tenancy_ocid        = var.tenancy_ocid
    region              = var.region
    mysql_regions       = var.mysql_regions
    mysql_compartments  = var.mysql_compartments
    ownership_kind      = var.ownership_kind
    ownership_namespace = var.ownership_namespace
    ownership_key       = var.ownership_key
    destinations        = var.destinations
    routes              = var.routes
    pagerduty_region    = var.pagerduty_region
    mode                = var.mode
    test_secret_ocid    = var.test_secret_ocid
    test_nonproduction  = var.test_nonproduction
    test_run            = var.test_run
  }
  topics = {
    intake   = oci_ons_notification_topic.intake.id
    operator = oci_ons_notification_topic.operator.id
  }
}
# Phase1 topics are inert: no subscriptions, announcement filters or publish IAM.
resource "oci_ons_notification_topic" "intake" {
  compartment_id = var.compartment_ocid
  name           = "${local.name}-intake"
  description    = "Reserved maintenance intake; PREVIEW installer creates no subscriptions."
}
resource "oci_ons_notification_topic" "operator" {
  compartment_id = var.compartment_ocid
  name           = "${local.name}-operator"
  description    = "Reserved operator notifications; PREVIEW installer cannot publish."
}
