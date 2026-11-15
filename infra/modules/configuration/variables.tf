variable "settings" {
  type = object({
    environment_name    = string
    tenancy_ocid        = string
    region              = string
    mysql_regions       = list(string)
    mysql_compartments  = list(string)
    ownership_kind      = string
    ownership_namespace = string
    ownership_key       = string
    destinations        = map(string)
    routes              = map(string)
    pagerduty_region    = string
    mode                = optional(string, "PREVIEW")
    test_secret_ocid    = optional(string, "")
    test_nonproduction  = optional(bool, false)
    test_run            = optional(number)
    operator_email      = optional(string, "")
    operator_test_run   = optional(number)
  })
  nullable    = false
  description = "Routing and explicit nonproduction test choices; no secret contents or maintenance-delivery authority."
}
variable "operator_subscription_id" {
  type        = string
  nullable    = false
  default     = ""
  description = "Deployment-owned EMAIL subscription; omitted when operator setup is not requested."
}
variable "catalog" {
  type        = object({ regions = map(string), mysqlServices = list(string) })
  nullable    = false
  description = "Independent deployment-owned region and service catalog."
}
variable "topics" {
  type        = object({ intake = string, operator = string })
  nullable    = false
  description = "Distinct deployment-owned regional topic identifiers."
}
variable "installation_id" {
  type        = string
  nullable    = false
  description = "Deployment-owned identity derived from the protected state bucket OCID."
  validation {
    condition     = can(regex("^b1-[a-f0-9]{64}$", var.installation_id))
    error_message = "Installation identity must be derived from the protected state bucket."
  }
}
