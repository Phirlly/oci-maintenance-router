# Resource Manager supplies the caller's managed authentication.
provider "oci" {
  region = var.region
}
provider "oci" {
  alias  = "home"
  region = local.home_region
}
data "oci_identity_region_subscriptions" "tenancy" {
  tenancy_id = var.tenancy_ocid
}
data "oci_objectstorage_namespace" "tenancy" {
  # Omitting compartment_id reads the caller tenancy without a tenancy-wide grant.
}
locals {
  home_region = one([for r in data.oci_identity_region_subscriptions.tenancy.region_subscriptions : r.region_name if r.is_home_region])
  catalog = {
    regions          = { for r in data.oci_identity_region_subscriptions.tenancy.region_subscriptions : r.region_name => lower(r.region_key) if r.state == "READY" }
    mysqlServices    = local.release.resourceServices.mysqldbsystem
    resourceServices = local.release.resourceServices
  }
  name = "omr-${var.environment_name}-${substr(sha256("${var.compartment_ocid}:${var.region}"), 0, 10)}"
}
