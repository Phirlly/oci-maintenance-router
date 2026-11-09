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
  description    = "PREVIEW configuration reads and deployment marker verification only."
  statements = [
    "Allow dynamic-group id ${oci_identity_dynamic_group.router.id} to manage objects in compartment id ${var.compartment_ocid} where all {target.bucket.name = '${oci_objectstorage_bucket.state.name}', request.permission = 'OBJECT_READ', any {target.object.name = 'configuration/router.json', target.object.name = 'configuration/catalog.json'}}",
    "Allow dynamic-group id ${oci_identity_dynamic_group.router.id} to manage objects in compartment id ${var.compartment_ocid} where all {target.bucket.name = '${oci_objectstorage_bucket.state.name}', target.object.name = 'deployment-checks/*', any {request.permission = 'OBJECT_READ', request.permission = 'OBJECT_CREATE', request.permission = 'OBJECT_OVERWRITE'}}",
  ]
}
