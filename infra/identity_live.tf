resource "oci_identity_policy" "live" {
  count          = local.mode == "LIVE" ? 1 : 0
  provider       = oci.home
  compartment_id = var.tenancy_ocid
  name           = "${local.name}-live"
  description    = "Guarded announcement processing: selected scope, destination secrets and retained state; no deletion or resource mutation."
  # A tenancy policy supports cross-compartment and explicitly selected root
  # resources. Conditions narrow metadata reads to the configured Cartesian scope.
  # ONS lacks exact-topic IAM conditions; the runtime binds the dedicated channel.
  statements = concat([
    "Allow dynamic-group id ${oci_identity_dynamic_group.router.id} to manage objects in compartment id ${var.compartment_ocid} where all {target.bucket.name = '${oci_objectstorage_bucket.state.name}', target.object.name = 'control/selected-baseline.json', any {request.permission = 'OBJECT_READ', request.permission = 'OBJECT_CREATE', request.permission = 'OBJECT_OVERWRITE'}}",
    "Allow dynamic-group id ${oci_identity_dynamic_group.router.id} to manage objects in compartment id ${var.compartment_ocid} where all {target.bucket.name = '${oci_objectstorage_bucket.state.name}', target.object.name = 'review/preparation/*', any {request.permission = 'OBJECT_READ', request.permission = 'OBJECT_CREATE', request.permission = 'OBJECT_OVERWRITE'}}",
    ], contains(local.resource_type_ids, "mysqldbsystem") ? [
    "Allow dynamic-group id ${oci_identity_dynamic_group.router.id} to read mysql-instances in tenancy where all {request.operation = 'GetDbSystem', request.permission = 'MYSQL_INSTANCE_READ', any {${join(", ", formatlist("target.compartment.id = '%s'", var.mysql_compartments))}}, any {${join(", ", [for region in var.mysql_regions : "request.region = '${local.catalog.regions[region]}'"])}}}",
    ] : [], contains(local.resource_type_ids, "instance") ? [
    "Allow dynamic-group id ${oci_identity_dynamic_group.router.id} to read instances in tenancy where all {request.operation = 'GetInstance', request.permission = 'INSTANCE_READ', any {${join(", ", formatlist("target.compartment.id = '%s'", var.mysql_compartments))}}, any {${join(", ", [for region in var.mysql_regions : "request.region = '${local.catalog.regions[region]}'"])}}}",
    ] : [], [
    "Allow dynamic-group id ${oci_identity_dynamic_group.router.id} to read secret-bundles in tenancy where all {request.permission = 'SECRET_BUNDLE_READ', any {${join(", ", formatlist("target.secret.id = '%s'", distinct(values(var.destinations))))}}}",
    "Allow dynamic-group id ${oci_identity_dynamic_group.router.id} to manage objects in compartment id ${var.compartment_ocid} where all {target.bucket.name = '${oci_objectstorage_bucket.state.name}', target.object.name = 'control/operator-reporting.json', any {request.permission = 'OBJECT_READ', request.permission = 'OBJECT_CREATE', request.permission = 'OBJECT_OVERWRITE'}}",
    "Allow dynamic-group id ${oci_identity_dynamic_group.router.id} to use ons-topics in compartment id ${var.compartment_ocid} where all {target.compartment.id = '${var.compartment_ocid}', request.region = '${local.catalog.regions[var.region]}', request.operation = 'PublishMessage', request.permission = 'ONS_TOPIC_PUBLISH'}",
    "Allow dynamic-group id ${oci_identity_dynamic_group.router.id} to use metrics in compartment id ${var.compartment_ocid} where all {target.metrics.namespace = 'maintenance_router', request.region = '${local.catalog.regions[var.region]}', request.operation = 'PostMetricData', request.permission = 'METRIC_WRITE'}",
  ])
}
