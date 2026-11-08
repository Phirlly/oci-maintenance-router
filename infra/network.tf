data "oci_core_services" "regional" {
  count = var.create_network ? 1 : 0
  filter {
    name   = "name"
    values = ["All .* Services In Oracle Services Network"]
    regex  = true
  }
  lifecycle {
    postcondition {
      condition     = length(self.services) == 1
      error_message = "Expected exactly one regional Oracle Services Network gateway service."
    }
  }
}
locals {
  regional_service = var.create_network ? one(data.oci_core_services.regional[0].services) : null
}
resource "oci_core_vcn" "router" {
  count          = var.create_network ? 1 : 0
  compartment_id = var.compartment_ocid
  display_name   = local.name
  cidr_blocks    = [var.network_cidr]
  dns_label      = "router"
  lifecycle {
    precondition {
      condition     = can(cidrnetmask(var.network_cidr)) && try(tonumber(split("/", var.network_cidr)[1]) >= 16 && tonumber(split("/", var.network_cidr)[1]) <= 28, false)
      error_message = "Dedicated network needs an IPv4 CIDR between /16 and /28."
    }
  }
}
resource "oci_core_service_gateway" "router" {
  count          = var.create_network ? 1 : 0
  compartment_id = var.compartment_ocid
  vcn_id         = oci_core_vcn.router[0].id
  display_name   = local.name
  services {
    service_id = local.regional_service.id
  }
}
resource "oci_core_route_table" "router" {
  count          = var.create_network ? 1 : 0
  compartment_id = var.compartment_ocid
  vcn_id         = oci_core_vcn.router[0].id
  display_name   = local.name
  route_rules {
    destination       = local.regional_service.cidr_block
    destination_type  = "SERVICE_CIDR_BLOCK"
    network_entity_id = oci_core_service_gateway.router[0].id
  }
}
resource "oci_core_security_list" "router" {
  count          = var.create_network ? 1 : 0
  compartment_id = var.compartment_ocid
  vcn_id         = oci_core_vcn.router[0].id
  display_name   = local.name
  egress_security_rules {
    destination      = local.regional_service.cidr_block
    destination_type = "SERVICE_CIDR_BLOCK"
    protocol         = "6"
    stateless        = false
    tcp_options {
      min = 443
      max = 443
    }
  }
}
resource "oci_core_subnet" "router" {
  count                      = var.create_network ? 1 : 0
  compartment_id             = var.compartment_ocid
  vcn_id                     = oci_core_vcn.router[0].id
  display_name               = local.name
  cidr_block                 = var.network_cidr
  dns_label                  = "private"
  prohibit_public_ip_on_vnic = true
  route_table_id             = oci_core_route_table.router[0].id
  security_list_ids          = [oci_core_security_list.router[0].id]
}
data "oci_core_subnet" "existing" {
  count     = var.create_network ? 0 : 1
  subnet_id = var.existing_subnet_ocid
  lifecycle {
    postcondition {
      condition     = self.prohibit_public_ip_on_vnic && self.availability_domain == "" && self.state == "AVAILABLE" && self.vcn_id == var.existing_vcn_ocid
      error_message = "Choose an AVAILABLE private regional subnet with working DNS and regional Object Storage HTTPS access."
    }
  }
}
