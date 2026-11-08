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
  })
  nullable    = false
  description = "Customer routing choices; no secret contents or activation authority."
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
