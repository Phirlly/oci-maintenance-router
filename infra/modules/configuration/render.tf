locals {
  s = var.settings
  configuration_json = jsonencode({
    schemaVersion    = 1
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
    mode            = "PREVIEW"
  })
  catalog_json = jsonencode(var.catalog)
  # Count UTF-8 octets, not Terraform's Unicode character/grapheme count.
  object_base64 = [base64encode(local.configuration_json), base64encode(local.catalog_json)]
  # Double encoding conservatively includes JSON escaping and RM serialization.
  input_base64    = { for name, value in local.s : name => base64encode(jsonencode(jsonencode(value))) }
  region_key      = lookup(var.catalog.regions, local.s.region, "")
  regional_suffix = "(${local.s.region}|${local.region_key})\\.([A-Za-z0-9_-]*\\.)?[A-Za-z0-9_-]+$"
}
