terraform {
  required_version = "= 1.5.7"
  required_providers {
    time = {
      source  = "hashicorp/time"
      version = "= 0.14.2"
    }
    oci = {
      source  = "oracle/oci"
      version = "= 9.12.1"
    }
  }
}
