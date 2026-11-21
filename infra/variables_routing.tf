variable "resource_types" {
  type        = list(string)
  nullable    = false
  default     = ["HeatWave MySQL"]
  description = "Resource types whose announcements are routed using the shared location and ownership-tag settings."
  validation {
    condition     = length(var.resource_types) > 0 && length(distinct(var.resource_types)) == length(var.resource_types) && alltrue([for choice in var.resource_types : contains(["HeatWave MySQL", "Compute instances"], choice)])
    error_message = "Resource types: select HeatWave MySQL, Compute instances, or both, without duplicates."
  }
}
locals {
  resource_type_ids = sort([for choice in var.resource_types : lookup({
    "HeatWave MySQL"    = "mysqldbsystem"
    "Compute instances" = "instance"
  }, choice, "")])
}

# Keep the original variable names so saved stack settings remain compatible.
variable "mysql_regions" {
  type        = list(string)
  nullable    = false
  description = "Explicit subscribed regions containing the selected resources."
}
variable "mysql_compartments" {
  type        = list(string)
  nullable    = false
  description = "Explicit resource compartments; child compartments are not implicit."
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
