locals {
  modern_setup    = var.settings.setup_action != null
  diagnostic_only = local.modern_setup && var.settings.mode == "TEST" && var.settings.operator_email == ""
  schema8         = local.modern_setup && !local.diagnostic_only
  setup_configuration = merge(local.common_configuration, {
    schemaVersion = 8
    setup = merge({ action = local.s.setup_action }, jsondecode(local.s.setup_action == "SEND_TEST" ? jsonencode({
      attempt       = local.s.setup_attempt
      secretId      = local.s.test_secret_ocid
      nonProduction = local.s.test_nonproduction
      }) : "{}"), jsondecode(local.s.setup_action == "CONFIRM" ? jsonencode({
      attempt       = local.s.setup_attempt
      pagerDutyCode = local.s.test_receipt_code
      operatorCode  = local.s.operator_receipt_code
    }) : "{}"))
    }, local.s.operator_email != "" ? {
    operatorChannel = { subscriptionId = var.operator_subscription_id, endpoint = local.s.operator_email }
  } : {})
  diagnostic_intent_required = (!local.schema8 && contains(["TEST", "LIVE"], local.s.mode)) || (local.schema8 && local.s.setup_action == "SEND_TEST")
}
