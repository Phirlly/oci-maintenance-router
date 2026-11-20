variable "operator_email" {
  type        = string
  nullable    = false
  default     = ""
  description = "Operator email required for Enabled routing. Apply while Paused with no setup action, confirm OCI's subscription email, then send the tests. Clearing it removes this stack's subscription."
  validation {
    condition = var.operator_email == "" || (
      can(regex("^[^@\\s\\p{Z}\\p{Cc}]+@[^@\\s\\p{Z}\\p{Cc}]+$", var.operator_email)) &&
      length(regexall(".", var.operator_email)) <= 512
    )
    error_message = "Operator email must be one address without whitespace or control characters, at most 512 Unicode characters; leave empty to omit operator setup."
  }
}
variable "operator_receipt_code" {
  type        = string
  nullable    = false
  default     = ""
  description = "Code from the received operator test email, used with the PagerDuty code by Confirm received tests."
}
