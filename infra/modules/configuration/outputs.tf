output "configuration_json" {
  description = "Validated PREVIEW, PAUSED or nonproduction TEST configuration; no secret contents."
  value       = local.configuration_json
  precondition {
    condition = local.s.mode != "TEST" || (
      can(regex("^(OMR-[0-9a-f]{32})?$", local.s.test_receipt_code)) &&
      can(regex("^(OMR-OP-[0-9a-f]{32})?$", local.s.operator_receipt_code))
    )
    error_message = "Receipt confirmation codes must match the received messages exactly: OMR- plus 32 lowercase hexadecimal characters for PagerDuty, OMR-OP- plus 32 for operator email. Leave empty to send a test."
  }
  precondition {
    condition = !local.receipt_confirmation || (
      local.s.test_receipt_code != "" &&
      (local.operator_test_enabled ? local.s.operator_receipt_code != "" : local.s.operator_receipt_code == "")
    )
    error_message = "Receipt confirmation requires the PagerDuty code and, when operator email is configured, its code too. Keep test run numbers unchanged. Clear the operator code if operator email was removed."
  }
  precondition {
    condition = local.s.operator_email == "" || (
      can(regex("^[^@\\s\\p{Z}\\p{Cc}]+@[^@\\s\\p{Z}\\p{Cc}]+$", local.s.operator_email)) &&
      length(regexall(".", local.s.operator_email)) <= 512
    )
    error_message = "Operator email must be one address without whitespace or control characters, at most 512 Unicode characters; leave empty to omit operator setup."
  }
  precondition {
    condition     = !local.operator_test_enabled || try(local.s.operator_test_run >= 1 && local.s.operator_test_run <= 2147483647 && floor(local.s.operator_test_run) == local.s.operator_test_run, false)
    error_message = "Operator test run number is required in TEST with an email: enter an integer from 1 to 2147483647. Keep unchanged when retrying; use a new number for another test or changed email/subscription."
  }
  precondition {
    condition     = !local.operator_test_enabled || (length(var.operator_subscription_id) <= 4096 && can(regex("^ocid1\\.onssubscription\\.oc1\\.${local.regional_suffix}", var.operator_subscription_id)))
    error_message = "Operator subscription must be created by this stack in the deployment region. Apply email setup in PREVIEW or PAUSED and confirm OCI's email before TEST."
  }
  precondition {
    condition     = contains(["PREVIEW", "PAUSED", "TEST"], local.s.mode)
    error_message = "Mode must be PREVIEW, PAUSED or TEST. Maintenance delivery is not enabled by this release."
  }
  precondition {
    condition     = local.s.mode != "TEST" || try(local.s.test_run >= 1 && local.s.test_run <= 2147483647 && floor(local.s.test_run) == local.s.test_run, false)
    error_message = "Test run number is required in TEST: enter an integer from 1 to 2147483647. Keep it unchanged to reuse the saved result; increase it to request another test."
  }
  precondition {
    condition     = local.s.mode != "TEST" || (length(local.s.test_secret_ocid) <= 4096 && can(regex("^ocid1\\.vaultsecret\\.oc1\\.${local.regional_suffix}", local.s.test_secret_ocid)))
    error_message = "Test secret must be an existing Vault secret in the stack's OCI region. Select its reference; never paste an integration key."
  }
  precondition {
    condition     = local.s.mode != "TEST" || local.s.test_nonproduction
    error_message = "Confirm the test secret belongs to a nonproduction PagerDuty service that may receive a test alert."
  }
  precondition {
    condition     = can(regex("^[a-z0-9][a-z0-9-]{0,31}$", local.s.environment_name))
    error_message = "Environment name must use 1–32 lowercase letters, digits or hyphens, beginning with a letter or digit."
  }
  precondition {
    condition     = can(regex("^ocid1\\.tenancy\\.oc1\\.\\.([A-Za-z0-9_-]*\\.)?[A-Za-z0-9_-]+$", local.s.tenancy_ocid))
    error_message = "An OC1 tenancy identifier is required."
  }
  precondition {
    condition     = contains(keys(var.catalog.regions), local.s.region)
    error_message = "Deployment region must be a READY subscribed OC1 region."
  }
  precondition {
    condition     = length(local.s.mysql_regions) > 0 && length(distinct(local.s.mysql_regions)) == length(local.s.mysql_regions) && alltrue([for region in local.s.mysql_regions : contains(keys(var.catalog.regions), region)])
    error_message = "MySQL regions must be nonempty, unique READY subscriptions."
  }
  precondition {
    condition     = length(local.s.mysql_compartments) > 0 && length(distinct(local.s.mysql_compartments)) == length(local.s.mysql_compartments) && alltrue([for id in local.s.mysql_compartments : id == local.s.tenancy_ocid || can(regex("^ocid1\\.compartment\\.oc1\\.\\.([A-Za-z0-9_-]*\\.)?[A-Za-z0-9_-]+$", id))])
    error_message = "MySQL compartments: select at least one compartment from this tenancy, including its root if needed, with no duplicates. Child compartments must be selected separately."
  }
  precondition {
    condition     = contains(["defined", "freeform"], local.s.ownership_kind) && can(regex("^[!-~]{1,100}$", local.s.ownership_key)) && !strcontains(local.s.ownership_key, ".") && (local.s.ownership_kind != "defined" || (can(regex("^[!-~]{1,100}$", local.s.ownership_namespace)) && !strcontains(local.s.ownership_namespace, ".")))
    error_message = "Ownership tag needs a defined/freeform kind and 1–100 printable ASCII characters without spaces or periods in its key and defined namespace."
  }
  precondition {
    condition     = length(local.s.destinations) > 0 && alltrue([for id in keys(local.s.destinations) : can(regex("^[a-z0-9][a-z0-9-]{0,31}$", id))])
    error_message = "Destinations: add at least one entry. Names must use 1–32 lowercase letters, digits or hyphens, starting with a letter or digit."
  }
  precondition {
    condition     = alltrue([for secret in values(local.s.destinations) : can(regex("^ocid1\\.vaultsecret\\.oc1\\.${local.regional_suffix}", secret))])
    error_message = "Destinations: select a Vault secret in the stack's OCI region using the picker on the right of each row. Save the variables and run Plan again. Never enter a PagerDuty integration key here."
  }
  precondition {
    condition     = length(local.s.routes) > 0 && alltrue([for owner, destination in local.s.routes : owner != "" && contains(keys(local.s.destinations), destination)])
    error_message = "Routes need nonempty owner values and destination names that exist in Destinations."
  }
  precondition {
    condition     = contains(["US", "EU"], local.s.pagerduty_region)
    error_message = "PagerDuty region must be US or EU."
  }
  precondition {
    condition     = alltrue([for encoded in local.object_base64 : length(encoded) * 3 / 4 - length(regexall("=", encoded)) <= 1048576])
    error_message = "Generated configuration and catalog must each fit within 1 MiB."
  }
  precondition {
    condition     = alltrue([for name, encoded in local.input_base64 : length(name) + length(encoded) * 3 / 4 - length(regexall("=", encoded)) <= 8192])
    error_message = "Routing settings exceed the 8192-byte RM variable budget including serialization; reduce the affected mapping or scope."
  }
}
output "catalog_json" {
  description = "Independent trusted catalog, not derived from selected routing scope."
  value       = local.catalog_json
}
output "receipt_confirmation" {
  description = "TEST confirmation intent from raw inputs; invalid pairs are rejected by configuration validation."
  value       = local.receipt_confirmation
}
