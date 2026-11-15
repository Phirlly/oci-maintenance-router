resource "oci_identity_dynamic_group" "router" {
  provider       = oci.home
  compartment_id = var.tenancy_ocid
  name           = local.name
  description    = "Only this maintenance-router Function."
  matching_rule  = "ALL {resource.type = 'fnfunc', resource.id = '${oci_functions_function.router.id}'}"
}
resource "oci_identity_policy" "router" {
  provider       = oci.home
  compartment_id = var.compartment_ocid
  name           = local.name
  description    = "Configuration and saved-review reads with guarded preparation state; no delete access."
  statements = [
    "Allow dynamic-group id ${oci_identity_dynamic_group.router.id} to manage objects in compartment id ${var.compartment_ocid} where all {target.bucket.name = '${oci_objectstorage_bucket.state.name}', request.permission = 'OBJECT_READ', any {target.object.name = 'configuration/router.json', target.object.name = 'configuration/catalog.json'}}",
    "Allow dynamic-group id ${oci_identity_dynamic_group.router.id} to inspect objects in compartment id ${var.compartment_ocid} where target.bucket.name = '${oci_objectstorage_bucket.state.name}'",
    "Allow dynamic-group id ${oci_identity_dynamic_group.router.id} to manage objects in compartment id ${var.compartment_ocid} where all {target.bucket.name = '${oci_objectstorage_bucket.state.name}', any {target.object.name = 'control/lease.json', target.object.name = 'control/prepared-history.json'}, any {request.permission = 'OBJECT_READ', request.permission = 'OBJECT_CREATE', request.permission = 'OBJECT_OVERWRITE'}}",
    "Allow dynamic-group id ${oci_identity_dynamic_group.router.id} to manage objects in compartment id ${var.compartment_ocid} where all {target.bucket.name = '${oci_objectstorage_bucket.state.name}', target.object.name = 'baselines/*', any {request.permission = 'OBJECT_READ', request.permission = 'OBJECT_CREATE'}}",
    "Allow dynamic-group id ${oci_identity_dynamic_group.router.id} to manage objects in compartment id ${var.compartment_ocid} where all {target.bucket.name = '${oci_objectstorage_bucket.state.name}', target.object.name = 'notices/*', any {request.permission = 'OBJECT_READ', request.permission = 'OBJECT_CREATE', request.permission = 'OBJECT_OVERWRITE'}}",
    "Allow dynamic-group id ${oci_identity_dynamic_group.router.id} to manage objects in compartment id ${var.compartment_ocid} where all {target.bucket.name = '${oci_objectstorage_bucket.state.name}', target.object.name = 'review/preparation/*', request.permission = 'OBJECT_READ'}",
  ]
}
resource "oci_identity_policy" "announcements" {
  provider       = oci.home
  compartment_id = var.tenancy_ocid
  name           = "${local.name}-announcements"
  description    = "Only this Function may read tenancy announcements for maintenance preparation."
  statements = [
    "Allow dynamic-group id ${oci_identity_dynamic_group.router.id} to read announcements in tenancy",
  ]
}
