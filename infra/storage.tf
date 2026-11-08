resource "oci_objectstorage_bucket" "state" {
  compartment_id = var.compartment_ocid
  namespace      = data.oci_objectstorage_namespace.tenancy.namespace
  name           = local.name
  access_type    = "NoPublicAccess"
  storage_tier   = "Standard"
  versioning     = "Enabled"
  lifecycle {
    prevent_destroy = true
  }
}
resource "oci_objectstorage_object" "configuration" {
  namespace                  = oci_objectstorage_bucket.state.namespace
  bucket                     = oci_objectstorage_bucket.state.name
  object                     = "configuration/router.json"
  content                    = module.configuration.configuration_json
  content_type               = "application/json"
  delete_all_object_versions = false
  # Default destroy-before-create preserves previous versions. Do not use CBD.
}
resource "oci_objectstorage_object" "catalog" {
  namespace                  = oci_objectstorage_bucket.state.namespace
  bucket                     = oci_objectstorage_bucket.state.name
  object                     = "configuration/catalog.json"
  content                    = module.configuration.catalog_json
  content_type               = "application/json"
  delete_all_object_versions = false
}
