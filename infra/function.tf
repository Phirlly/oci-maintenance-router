locals {
  release           = jsondecode(file("${path.module}/release.json"))
  function_manifest = jsondecode(file("${path.module}/function-manifest.json"))
  archive_path      = "${path.module}/function.zip"
  release_sha256    = sha256(jsonencode({ release = local.release, archiveSha256 = filebase64sha256(local.archive_path) }))
  runtime_versions  = flatten([for collection in data.oci_functions_functions_runtime_versions.approved.functions_runtime_version_collection : collection.items])
  approved_runtime_ids = [for runtime in local.runtime_versions : runtime.id
    if runtime.display_name == local.release.runtime.version && runtime.os_version == local.release.runtime.osVersion &&
  runtime.state == "ACTIVE" && contains(runtime.supported_architectures, local.release.runtime.architecture)]
  bootstrap = jsonencode({
    tenancyId            = var.tenancy_ocid
    metricsCompartmentId = var.compartment_ocid
    region               = var.region
    namespace            = oci_objectstorage_bucket.state.namespace
    bucket               = oci_objectstorage_bucket.state.name
    configurationVersion = oci_objectstorage_object.configuration.version_id
    catalogVersion       = oci_objectstorage_object.catalog.version_id
    configurationSha256  = sha256(module.configuration.configuration_json)
    catalogSha256        = sha256(module.configuration.catalog_json)
    releaseSha256        = local.release_sha256
  })
  bootstrap_base64 = base64encode(local.bootstrap)
}
data "oci_functions_functions_runtime_versions" "approved" {
  functions_runtime_name = local.release.runtime.name
  display_name           = local.release.runtime.version
  os_version             = local.release.runtime.osVersion
  state                  = "ACTIVE"
}
resource "oci_functions_application" "router" {
  compartment_id = var.compartment_ocid
  display_name   = local.name
  shape          = "GENERIC_X86"
  subnet_ids     = var.create_network ? [oci_core_subnet.router[0].id] : [data.oci_core_subnet.existing[0].id]
  logging {
    line_format = "JSON"
  }
}
resource "oci_functions_function" "router" {
  application_id                   = oci_functions_application.router.id
  display_name                     = "maintenance-router"
  memory_in_mbs                    = "256"
  timeout_in_seconds               = 300
  detached_mode_timeout_in_seconds = 300
  failure_destination {
    kind     = local.mode == "LIVE" ? "NOTIFICATION" : "NONE"
    topic_id = local.mode == "LIVE" ? oci_ons_notification_topic.operator.id : null
  }
  success_destination {
    kind = "NONE"
  }
  depends_on = [oci_identity_policy.detached_failure]
  config     = { ROUTER_BOOTSTRAP = local.bootstrap }
  source_details {
    source_type = "ARCHIVE"
    archive_source_details {
      archive_source_type = "DIRECT_ARCHIVE"
      archive_file        = sensitive(filebase64(local.archive_path))
    }
    runtime_config {
      functions_runtime_name       = local.release.runtime.name
      runtime_config_type          = "MANUAL"
      functions_runtime_version_id = try(one(local.approved_runtime_ids), null)
    }
  }
  lifecycle {
    precondition {
      condition     = length(local.approved_runtime_ids) == 1
      error_message = "The exact approved ACTIVE X86 runtime is unavailable or ambiguous in this region; use a verified release/region."
    }
    precondition {
      condition     = local.function_manifest.schemaVersion == 1 && local.function_manifest.purpose == "maintenance-router-runtime" && local.function_manifest.architecture == "X86" && local.function_manifest.operatingSystem == "linux" && local.function_manifest.archiveSha256 == filesha256(local.archive_path) && local.function_manifest.archiveSha256Base64 == filebase64sha256(local.archive_path)
      error_message = "Bundled Function archive does not match its approved manifest."
    }
    precondition {
      condition     = length("ROUTER_BOOTSTRAP") + length(local.bootstrap_base64) * 3 / 4 - length(regexall("=", local.bootstrap_base64)) <= 4096
      error_message = "Function bootstrap exceeds the 4096-byte aggregate environment budget."
    }
  }
}
data "oci_functions_function" "archive" {
  function_id = oci_functions_function.router.id
  # Read after updates: the managed resource retains its old checksum during Plan.
  depends_on = [oci_functions_function.router]
  lifecycle {
    # Verify exact bytes in the documented Base64 or observed hexadecimal encoding.
    postcondition {
      condition = try(
        self.source_details[0].source_code_sha256 == filesha256(local.archive_path) ||
        self.source_details[0].source_code_sha256 == filebase64sha256(local.archive_path),
        false
      )
      error_message = "OCI did not report the bundled Function archive checksum."
    }
  }
}
