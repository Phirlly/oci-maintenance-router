# oci-maintenance-router

An OCI maintenance-notification router intended to send relevant MySQL
maintenance notices to the PagerDuty service responsible for each affected
resource, using current ownership tags instead of a manually maintained list
of resource IDs.

Status: implementation in progress. Deployment-settings validation, notice
normalization with stable revision IDs, ownership-based routing decisions, and
recipient-specific message drafts are implemented in Go. Customer scope, tags,
destinations and environment settings come from validated configuration. Unresolved
resources and messages exceeding application limits remain visible for review.
There is no runnable router or deployment package yet.

The intended installation is a **Deploy to OCI** button and a Resource Manager
form for resource scope, ownership tags and PagerDuty destination secret
references. Customers will not need to compile code or build a container.

The initial scope is notification routing. Resource modification and automatic
alarm suppression are outside that scope.
