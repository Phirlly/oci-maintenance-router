variable "tenancy_ocid" {
  type        = string
  nullable    = false
  description = "Stack tenancy; populated by Resource Manager."
}
variable "compartment_ocid" {
  type        = string
  nullable    = false
  description = "Compartment for the router and its owned resources."
}
variable "region" {
  type        = string
  nullable    = false
  description = "Stack deployment region; populated by Resource Manager."
}
variable "environment_name" {
  type        = string
  nullable    = false
  description = "Stable unique environment name; changing it replaces protected storage."
}
