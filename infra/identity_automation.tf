resource "oci_identity_dynamic_group" "schedule" {
  provider       = oci.home
  compartment_id = var.tenancy_ocid
  name           = "${local.name}-schedule"
  description    = "Only this router's reconciliation schedule."
  matching_rule  = "ALL {resource.type = 'resourceschedule', resource.id = '${oci_resource_scheduler_schedule.reconcile.id}'}"
}
resource "oci_identity_policy" "schedule" {
  provider       = oci.home
  compartment_id = var.compartment_ocid
  name           = "${local.name}-schedule"
  description    = "The exact schedule may read and invoke only this router Function."
  statements = [
    "Allow dynamic-group id ${oci_identity_dynamic_group.schedule.id} to use fn-invocation in compartment id ${var.compartment_ocid} where all {target.function.id = '${oci_functions_function.router.id}', request.region = '${local.catalog.regions[var.region]}', request.operation = 'InvokeFunction', request.permission = 'FN_INVOCATION'}",
    "Allow dynamic-group id ${oci_identity_dynamic_group.schedule.id} to read fn-function in compartment id ${var.compartment_ocid} where all {target.function.id = '${oci_functions_function.router.id}', request.region = '${local.catalog.regions[var.region]}', request.operation = 'GetFunction', request.permission = 'FN_FUNCTION_READ'}",
  ]
}
resource "oci_identity_policy" "detached_failure" {
  count          = local.mode == "LIVE" ? 1 : 0
  provider       = oci.home
  compartment_id = var.compartment_ocid
  name           = "${local.name}-detached-failure"
  description    = "This application's platform failure records may reach the operator channel."
  # Detached results use fnapp, not the runtime fnfunc principal. ONS does not
  # document an exact-topic condition; the Function destination binds the topic.
  statements = [
    "Allow any-user to use ons-topics in compartment id ${var.compartment_ocid} where all {request.principal.type = 'fnapp', request.principal.id = '${oci_functions_application.router.id}', request.principal.compartment.id = '${var.compartment_ocid}', request.region = '${local.catalog.regions[var.region]}', request.operation = 'PublishMessage', request.permission = 'ONS_TOPIC_PUBLISH'}",
  ]
}
