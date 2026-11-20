resource "oci_identity_policy" "diagnostic" {
  count          = contains(["TEST", "LIVE"], local.mode) ? 1 : 0
  provider       = oci.home
  compartment_id = var.tenancy_ocid
  name           = "${local.name}-diagnostic"
  description    = "Installation proof reads when enabled; exact attempt access during explicit setup."
  statements = (local.mode == "LIVE" || local.receipt_confirmation) ? [
    "Allow dynamic-group id ${oci_identity_dynamic_group.router.id} to read objects in compartment id ${var.compartment_ocid} where all {target.bucket.name = '${oci_objectstorage_bucket.state.name}', target.object.name = 'control/diagnostics/${local.installation_id}/${local.mode == "LIVE" ? "*" : "${local.diagnostic_request_id}.json"}', request.permission = 'OBJECT_READ'}",
    ] : [
    "Allow dynamic-group id ${oci_identity_dynamic_group.router.id} to read secret-bundles in tenancy where all {target.secret.id = '${var.test_secret_ocid}', request.permission = 'SECRET_BUNDLE_READ'}",
    "Allow dynamic-group id ${oci_identity_dynamic_group.router.id} to manage objects in compartment id ${var.compartment_ocid} where all {target.bucket.name = '${oci_objectstorage_bucket.state.name}', target.object.name = 'control/diagnostics/${local.installation_id}/${local.diagnostic_request_id}.json', any {request.permission = 'OBJECT_READ', request.permission = 'OBJECT_CREATE', request.permission = 'OBJECT_OVERWRITE'}}",
  ]
}
