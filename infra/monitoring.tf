locals {
  operational_alarms = {
    missing-scan = {
      namespace = "maintenance_router"
      query     = "CompletedScan[2h]{installationId=\"${local.installation_id}\"}.groupBy(installationId).absent(72h)"
      severity  = "ERROR"
      body      = "No completed router scan was observed in the query window. Check Function logs, permissions and the reconciliation schedule. This can fire before the first successful scan. OK/RESET alone is not recovery: verify a recent actual CompletedScan datapoint."
    }
    review-work = {
      namespace = "maintenance_router"
      query     = "ReviewIssues[2h]{installationId=\"${local.installation_id}\"}.groupBy(installationId).last() > 0"
      severity  = "ERROR"
      body      = "The router has saved work requiring operator review. Inspect the operator summary and retained notice status. This is a saved issue count, not proof that every notice was delivered."
    }
    report-failure = {
      namespace = "maintenance_router"
      query     = "OperatorReportFailed[2h]{installationId=\"${local.installation_id}\"}.groupBy(installationId).last() > 0"
      severity  = "ERROR"
      body      = "An operator summary could not be confirmed. Check Function logs, Notifications permissions and the confirmed email subscription. A pending retry does not establish recovery; retained issue summaries remain available."
    }
    function-error = {
      namespace = "oci_faas"
      query     = "FunctionResponseCount[5m]{resourceId=\"${oci_functions_function.router.id}\",responseType=\"Error\"}.grouping().sum() > 0"
      severity  = "ERROR"
      body      = "A router Function invocation failed. Check its safe result and Function logs. Previously accepted PagerDuty work remains retained; do not reset or delete state to retry."
    }
  }
}

resource "oci_monitoring_alarm" "router" {
  for_each = local.operational_alarms

  compartment_id                                = var.compartment_ocid
  display_name                                  = "${local.name}-${each.key}"
  destinations                                  = [oci_ons_notification_topic.operator.id]
  is_enabled                                    = var.mode == "LIVE"
  metric_compartment_id                         = var.compartment_ocid
  metric_compartment_id_in_subtree              = false
  namespace                                     = each.value.namespace
  query                                         = each.value.query
  severity                                      = each.value.severity
  body                                          = each.value.body
  message_format                                = "ONS_OPTIMIZED"
  is_notifications_per_metric_dimension_enabled = false
  resolution                                    = "1m"
  pending_duration                              = "PT1M"
  evaluation_slack_duration                     = "PT3M"
  repeat_notification_duration                  = "PT2H"
}
