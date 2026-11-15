variable "closure_review_code" {
  type        = string
  nullable    = false
  default     = ""
  description = "Exact code from a completed saved-target review. Used only when closure is confirmed."
}
variable "closure_request_number" {
  type        = number
  default     = null
  description = "Explicit closure number, 1 to 2147483647. Keep number and code unchanged when retrying; use a new number for new intent. History is scoped to each notice."
}
variable "closure_confirm" {
  type        = bool
  nullable    = false
  default     = false
  description = "Explicitly close the reviewed saved work without sending, in PAUSED. Clear and Apply to remove closure intent; PAUSED alone does not cancel it."
}
