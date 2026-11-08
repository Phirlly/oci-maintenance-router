variable "mysql_regions" {
  type        = list(string)
  nullable    = false
  description = "Explicit subscribed regions to include in MySQL routing."
}
variable "mysql_compartments" {
  type        = list(string)
  nullable    = false
  description = "Explicit MySQL compartments; child compartments are not implicit."
}
variable "ownership_kind" {
  type        = string
  nullable    = false
  description = "Select defined or freeform ownership tags."
  default     = "defined"
}
variable "ownership_namespace" {
  type        = string
  nullable    = false
  description = "Defined-tag namespace; ignored for freeform tags."
  default     = ""
}
variable "ownership_key" {
  type        = string
  nullable    = false
  description = "Ownership tag key."
}
variable "destinations" {
  type        = map(string)
  nullable    = false
  description = "Stable destination name to existing regional Vault secret OCID; never paste a key."
}
variable "routes" {
  type        = map(string)
  nullable    = false
  description = "Exact NFC owner tag value to a destination name."
}
variable "pagerduty_region" {
  type        = string
  nullable    = false
  description = "PagerDuty account region, US or EU."
  default     = "US"
}
variable "secret_compartment_ocid" {
  type        = string
  nullable    = false
  description = "Compartment used by the form's Vault selector."
}
