variable "routing_state" {
  type        = string
  nullable    = false
  default     = "Paused"
  description = "Paused prepares history without maintenance delivery. Enabled requires saved confirmation of both setup tests."
  validation {
    condition     = contains(["Paused", "Enabled"], var.routing_state)
    error_message = "Routing must be Paused or Enabled."
  }
}
variable "setup_action" {
  type        = string
  nullable    = false
  default     = "None"
  description = "Explicit setup action while routing is Paused. Confirmation sends no test."
  validation {
    condition     = contains(["None", "Send test", "Confirm received tests"], var.setup_action)
    error_message = "Choose None, Send test or Confirm received tests."
  }
}
variable "test_secret_compartment_ocid" {
  type        = string
  nullable    = false
  default     = ""
  description = "Compartment used by the separate test-secret picker."
}
variable "test_secret_ocid" {
  type        = string
  nullable    = false
  default     = ""
  description = "Existing regional Vault secret holding the nonproduction Events API v2 integration key."
}
variable "test_nonproduction" {
  type        = bool
  nullable    = false
  default     = false
  description = "Explicit confirmation that the selected service may receive a nonproduction test alert."
}
variable "setup_attempt" {
  type        = number
  default     = null
  description = "Explicit positive setup attempt. Keep unchanged to retry or confirm; increase only for another test. Unused when Setup action is None."
}
variable "test_receipt_code" {
  type        = string
  nullable    = false
  default     = ""
  description = "Code from the received PagerDuty test, used only by Confirm received tests."
}
