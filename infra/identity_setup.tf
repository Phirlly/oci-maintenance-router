locals {
  # Match the runtime's environment and attempt identities exactly. No request
  # numbers are needed for Enabled routing: the saved head selects its proofs.
  setup_environment_id  = sha256(jsonencode([var.tenancy_ocid, var.region, oci_objectstorage_bucket.state.namespace, oci_objectstorage_bucket.state.name]))
  diagnostic_request_id = local.diagnostic_only ? jsondecode(module.configuration.configuration_json).diagnostic.requestId : "sd1-${sha256(jsonencode(["setup-diagnostic-v1", local.setup_environment_id, local.installation_id, var.setup_attempt]))}"
  operator_request_id   = "so1-${sha256(jsonencode(["setup-operator-v1", local.setup_environment_id, local.installation_id, var.setup_attempt]))}"
}

resource "oci_identity_policy" "setup" {
  count          = local.setup_paired || local.mode == "LIVE" ? 1 : 0
  provider       = oci.home
  compartment_id = var.compartment_ocid
  name           = "${local.name}-setup"
  description    = "Selected setup head and immutable confirmation; writes only during explicit paired setup."
  statements = local.mode == "LIVE" ? [
    "Allow dynamic-group id ${oci_identity_dynamic_group.router.id} to read objects in compartment id ${var.compartment_ocid} where all {target.bucket.name = '${oci_objectstorage_bucket.state.name}', target.object.name = 'control/setup-current/${local.installation_id}.json', request.permission = 'OBJECT_READ'}",
    "Allow dynamic-group id ${oci_identity_dynamic_group.router.id} to read objects in compartment id ${var.compartment_ocid} where all {target.bucket.name = '${oci_objectstorage_bucket.state.name}', target.object.name = 'control/setup-confirmations/${local.installation_id}/*', request.permission = 'OBJECT_READ'}",
    ] : local.receipt_confirmation ? [
    "Allow dynamic-group id ${oci_identity_dynamic_group.router.id} to manage objects in compartment id ${var.compartment_ocid} where all {target.bucket.name = '${oci_objectstorage_bucket.state.name}', target.object.name = 'control/setup-current/${local.installation_id}.json', any {request.permission = 'OBJECT_READ', request.permission = 'OBJECT_OVERWRITE'}}",
    "Allow dynamic-group id ${oci_identity_dynamic_group.router.id} to manage objects in compartment id ${var.compartment_ocid} where all {target.bucket.name = '${oci_objectstorage_bucket.state.name}', target.object.name = 'control/setup-confirmations/${local.installation_id}/*', any {request.permission = 'OBJECT_READ', request.permission = 'OBJECT_CREATE'}}",
    ] : [
    "Allow dynamic-group id ${oci_identity_dynamic_group.router.id} to manage objects in compartment id ${var.compartment_ocid} where all {target.bucket.name = '${oci_objectstorage_bucket.state.name}', target.object.name = 'control/setup-current/${local.installation_id}.json', any {request.permission = 'OBJECT_READ', request.permission = 'OBJECT_CREATE', request.permission = 'OBJECT_OVERWRITE'}}",
  ]
}
