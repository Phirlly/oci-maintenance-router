# oci-maintenance-router

An OCI maintenance-notification router intended to send relevant MySQL
maintenance notices to the PagerDuty service responsible for each affected
resource, using current ownership tags instead of a manually maintained list
of resource IDs.

Status: repository preparation only. There is no runnable router, installation
command, deployment package, or supported runtime yet.

The initial scope is notification routing. Resource modification and automatic
alarm suppression are outside that scope.
