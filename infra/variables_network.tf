variable "create_network" {
  type        = bool
  nullable    = false
  description = "Create a dedicated private network; false references an existing subnet."
  default     = true
}
variable "network_cidr" {
  type        = string
  nullable    = false
  description = "IPv4 CIDR for the dedicated VCN and subnet."
  default     = "10.77.0.0/24"
}
variable "network_compartment_ocid" {
  type        = string
  nullable    = false
  description = "Compartment containing an existing network; used only by the form."
  default     = ""
}
variable "existing_subnet_ocid" {
  type        = string
  nullable    = false
  description = "Existing private regional subnet with DNS and Object Storage HTTPS access."
  default     = ""
}
variable "existing_vcn_ocid" {
  type        = string
  nullable    = false
  description = "VCN containing the selected existing subnet."
  default     = ""
}
