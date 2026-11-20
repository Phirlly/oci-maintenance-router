resource "oci_identity_policy" "operator" {
  count          = (local.operator_test_enabled || local.mode == "LIVE") ? 1 : 0
  provider       = oci.home
  compartment_id = var.compartment_ocid
  name           = "${local.name}-operator"
  description    = "Current operator channel and exact proof; publication only for an explicit TEST."
  # Notifications supports general IAM variables, not an exact-topic OCID
  # condition. The runtime separately binds the dedicated topic/subscription.
  statements = concat([
    "Allow dynamic-group id ${oci_identity_dynamic_group.router.id} to use ons-topics in compartment id ${var.compartment_ocid} where all {target.compartment.id = '${var.compartment_ocid}', request.region = '${local.catalog.regions[var.region]}', request.operation = 'GetTopic', request.permission = 'ONS_TOPIC_READ'}",
    "Allow dynamic-group id ${oci_identity_dynamic_group.router.id} to use ons-topics in compartment id ${var.compartment_ocid} where all {target.compartment.id = '${var.compartment_ocid}', request.region = '${local.catalog.regions[var.region]}', request.operation = 'GetSubscription', request.permission = 'ONS_TOPIC_SUBSCRIBE'}",
    ], (local.mode == "LIVE" || local.receipt_confirmation) ? [
    "Allow dynamic-group id ${oci_identity_dynamic_group.router.id} to read objects in compartment id ${var.compartment_ocid} where all {target.bucket.name = '${oci_objectstorage_bucket.state.name}', target.object.name = 'control/operator-challenges/${local.installation_id}/${local.mode == "LIVE" ? "*" : "${local.operator_request_id}.json"}', request.permission = 'OBJECT_READ'}",
    ] : [
    "Allow dynamic-group id ${oci_identity_dynamic_group.router.id} to use ons-topics in compartment id ${var.compartment_ocid} where all {target.compartment.id = '${var.compartment_ocid}', request.region = '${local.catalog.regions[var.region]}', request.operation = 'PublishMessage', request.permission = 'ONS_TOPIC_PUBLISH'}",
    "Allow dynamic-group id ${oci_identity_dynamic_group.router.id} to manage objects in compartment id ${var.compartment_ocid} where all {target.bucket.name = '${oci_objectstorage_bucket.state.name}', target.object.name = 'control/operator-challenges/${local.installation_id}/${local.operator_request_id}.json', any {request.permission = 'OBJECT_READ', request.permission = 'OBJECT_CREATE', request.permission = 'OBJECT_OVERWRITE'}}",
  ])
}
