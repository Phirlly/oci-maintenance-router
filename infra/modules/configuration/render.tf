locals {
  # Old module callers retain schema 7; the root always supplies explicit setup.
  s = merge(var.settings, {
    test_run              = local.modern_setup ? var.settings.setup_attempt : var.settings.test_run
    operator_test_run     = local.modern_setup ? var.settings.setup_attempt : var.settings.operator_test_run
    test_receipt_code     = !local.modern_setup || var.settings.setup_action == "CONFIRM" ? var.settings.test_receipt_code : ""
    operator_receipt_code = !local.modern_setup || var.settings.setup_action == "CONFIRM" ? var.settings.operator_receipt_code : ""
  })
  receipt_confirmation = local.s.mode == "TEST" && (local.modern_setup ? local.s.setup_action == "CONFIRM" : (local.s.test_receipt_code != "" || local.s.operator_receipt_code != ""))
  # Missing run is never defaulted into authority to send another test.
  diagnostic_request_id = local.s.test_run == null ? null : (local.diagnostic_only ? "st1-${sha256(jsonencode(["setup-diagnostic-only-v1", var.installation_id, local.s.test_run]))}" : "d1-${sha256(jsonencode(["diagnostic-request-v1", var.installation_id, local.s.test_run]))}")
  operator_test_enabled = contains(["TEST", "LIVE"], local.s.mode) && local.s.operator_email != ""
  operator_request_id   = local.s.operator_test_run == null ? null : "o1-${sha256(jsonencode(["operator-request-v1", var.installation_id, local.s.operator_test_run]))}"
  common_configuration = merge({
    installationId   = var.installation_id
    preparation      = { initialize = true }
    environmentName  = local.s.environment_name
    tenancyId        = local.s.tenancy_ocid
    deploymentRegion = local.s.region
    regionMap        = { for region in distinct(local.s.mysql_regions) : region => lookup(var.catalog.regions, region, "") }
    compartmentIds   = sort(local.s.mysql_compartments)
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
  }, jsondecode(local.selected_scope ? jsonencode({ resourceTypes = sort(local.s.resource_types) }) : jsonencode({ mysqlServices = sort(var.catalog.mysqlServices) })), local.closure_enabled ? { recovery = local.closure_request } : {}, local.recovery_enabled ? { recovery = local.recovery_request } : {})
  legacy_configuration = merge(local.common_configuration, { schemaVersion = 7 }, contains(["TEST", "LIVE"], local.s.mode) ? {
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
  } : {})
  configuration_json = local.saved_setup ? jsonencode(local.setup_configuration) : jsonencode(local.legacy_configuration)
  # Strict runtime schemas reject extra/null attributes; never encode the input object.
  catalog_json = local.selected_scope ? jsonencode({ schemaVersion = 1, regions = var.catalog.regions, resourceServices = var.catalog.resourceServices }) : jsonencode({ regions = var.catalog.regions, mysqlServices = var.catalog.mysqlServices })
  # Count UTF-8 octets, not Terraform's Unicode character/grapheme count.
  object_base64 = [base64encode(local.configuration_json), base64encode(local.catalog_json)]
  # Double encoding conservatively includes JSON escaping and RM serialization.
  input_base64    = { for name, value in local.s : name => base64encode(jsonencode(jsonencode(value))) }
  region_key      = lookup(var.catalog.regions, local.s.region, "")
  regional_suffix = "(${local.s.region}|${local.region_key})\\.([A-Za-z0-9_-]*\\.)?[A-Za-z0-9_-]+$"
}
