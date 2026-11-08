# oci-maintenance-router

An OCI maintenance-notification router intended to send relevant MySQL
maintenance notices to the PagerDuty service responsible for each affected
resource, using current ownership tags instead of a manually maintained list
of resource IDs.

Status: implementation in progress. Deployment-settings validation and notice
normalization with stable revision IDs are implemented in Go. There is no
runnable router or deployment package yet.

The intended installation is a **Deploy to OCI** button and a Resource Manager
form for resource scope, ownership tags and PagerDuty destination secret
references. Customers will not need to compile code or build a container.

The initial scope is notification routing. Resource modification and automatic
alarm suppression are outside that scope.
