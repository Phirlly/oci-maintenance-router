locals {
  # Paused installation prepares history; explicit closure remains quiescence-only.
  mode = var.routing_state == "Enabled" ? "LIVE" : (var.setup_action != "None" ? "TEST" : (var.closure_confirm ? "PAUSED" : "PREVIEW"))
  setup_action = lookup({
    "None"                   = "NONE"
    "Send test"              = "SEND_TEST"
    "Confirm received tests" = "CONFIRM"
  }, var.setup_action, "INVALID")
  diagnostic_only = local.mode == "TEST" && var.operator_email == ""
  setup_paired    = local.mode == "TEST" && var.operator_email != ""
}
