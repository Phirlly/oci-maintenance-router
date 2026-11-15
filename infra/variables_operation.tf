variable "mode" {
  type        = string
  nullable    = false
  default     = "PREVIEW"
  description = "PREVIEW prepares history; PAUSED stops processing and permits confirmed closure, while hourly checks continue without sending; TEST sends or confirms nonproduction tests; LIVE requires both saved proofs and prepared history."
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
variable "test_run" {
  type        = number
  default     = null
  description = "Explicit positive test number in TEST and LIVE; unchanged reuses the saved result, increase for another test in TEST."
}
variable "test_receipt_code" {
  type        = string
  nullable    = false
  default     = ""
  description = "Confirmation code from the received PagerDuty test, required in LIVE. Keep test numbers unchanged; supply every configured receipt code together."
}
