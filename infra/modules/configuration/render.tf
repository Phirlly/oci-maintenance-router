locals {
  s = var.settings
  # Use raw inputs: an orphan operator code must never fall back to sending.
  receipt_confirmation = local.s.mode == "TEST" && (local.s.test_receipt_code != "" || local.s.operator_receipt_code != "")
  # Missing run is never defaulted into authority to send another test.
  diagnostic_request_id = local.s.test_run == null ? null : "d1-${sha256(jsonencode(["diagnostic-request-v1", var.installation_id, local.s.test_run]))}"
  operator_test_enabled = local.s.mode == "TEST" && local.s.operator_email != ""
  operator_request_id   = local.s.operator_test_run == null ? null : "o1-${sha256(jsonencode(["operator-request-v1", var.installation_id, local.s.operator_test_run]))}"
  configuration_json = jsonencode(merge({
    schemaVersion    = 7
    installationId   = var.installation_id
    preparation      = { initialize = true }
    environmentName  = local.s.environment_name
    tenancyId        = local.s.tenancy_ocid
    deploymentRegion = local.s.region
    regionMap        = { for region in distinct(local.s.mysql_regions) : region => lookup(var.catalog.regions, region, "") }
    compartmentIds   = sort(local.s.mysql_compartments)
    mysqlServices    = sort(var.catalog.mysqlServices)
    ownership = merge(
      { kind = local.s.ownership_kind, key = local.s.ownership_key },
      local.s.ownership_kind == "defined" ? { namespace = local.s.ownership_namespace } : {}
    )
    destinations    = [for id, secret in local.s.destinations : { id = id, secretId = secret }]
    routes          = [for owner, destination in local.s.routes : { ownerValue = owner, destinationId = destination }]
    pagerDutyRegion = local.s.pagerduty_region
    intakeTopicId   = var.topics.intake
    operatorTopicId = var.topics.operator
    mode            = local.s.mode
    }, local.s.mode == "TEST" ? {
    diagnostic = merge({
      requestId     = local.diagnostic_request_id
      secretId      = local.s.test_secret_ocid
      nonProduction = local.s.test_nonproduction
    }, local.s.test_receipt_code != "" ? { receiptCode = local.s.test_receipt_code } : {})
    } : {}, local.operator_test_enabled ? {
    operatorTest = merge({
      requestId      = local.operator_request_id
      subscriptionId = var.operator_subscription_id
      endpoint       = local.s.operator_email
    }, local.s.operator_receipt_code != "" ? { receiptCode = local.s.operator_receipt_code } : {})
  } : {}, local.closure_enabled ? { recovery = local.closure_request } : {}))
  catalog_json = jsonencode(var.catalog)
  # Count UTF-8 octets, not Terraform's Unicode character/grapheme count.
  object_base64 = [base64encode(local.configuration_json), base64encode(local.catalog_json)]
  # Double encoding conservatively includes JSON escaping and RM serialization.
  input_base64    = { for name, value in local.s : name => base64encode(jsonencode(jsonencode(value))) }
  region_key      = lookup(var.catalog.regions, local.s.region, "")
  regional_suffix = "(${local.s.region}|${local.region_key})\\.([A-Za-z0-9_-]*\\.)?[A-Za-z0-9_-]+$"
}
