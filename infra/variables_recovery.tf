variable "recovery_review_code" {
  type        = string
  nullable    = false
  default     = ""
  description = "Exact final code from review-recovery for the selected retry or revision approval. Used only when recovery is confirmed."
}
variable "recovery_request_number" {
  type        = number
  default     = null
  description = "Explicit recovery number, 1 to 2147483647. Keep number and code unchanged when retrying; use a new number for new intent. History is scoped to each notice."
}
variable "recovery_confirm" {
  type        = bool
  nullable    = false
  default     = false
  description = "Authorize reviewed sending recovery in LIVE. Clear and Apply to remove intent; already approved batches remain authorized."
}
