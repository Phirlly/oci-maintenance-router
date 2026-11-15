resource "oci_logging_log_group" "router" {
  compartment_id = var.compartment_ocid
  display_name   = "${local.name}-logs"
  description    = "Maintenance router invocation outcomes."
}

resource "oci_logging_log" "router" {
  display_name       = "${local.name}-invocations"
  log_group_id       = oci_logging_log_group.router.id
  log_type           = "SERVICE"
  is_enabled         = true
  retention_duration = 30
  configuration {
    source {
      service     = "functions"
      resource    = oci_functions_application.router.id
      category    = "invoke"
      source_type = "OCISERVICE"
    }
  }
}
