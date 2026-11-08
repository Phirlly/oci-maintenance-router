# oci-maintenance-router

An OCI maintenance-notification router intended to send relevant MySQL
maintenance notices to the PagerDuty service responsible for each affected
resource, using current ownership tags instead of a manually maintained list
of resource IDs.

Status: implementation in progress. Deployment-settings validation, notice
normalization with stable revision IDs, ownership-based routing decisions,
recipient-specific message drafts, delivery-state/recovery rules, and a bounded
OCI Object Storage adapter are implemented in Go. Saved-record encoding preserves
messages and retry history across reloads. The storage adapter supports conditional
writes and reports conflicting or uncertain outcomes; it has been tested against
local TLS fixtures. Complete notice-state coordination and PagerDuty transport
are still to be connected and tested in OCI. Customer scope, tags,
destinations and environment settings come from validated configuration. Unresolved
resources and messages exceeding application limits remain visible for review.
There is no runnable router or deployment package yet.

The intended installation is a **Deploy to OCI** button and a Resource Manager
form for resource scope, ownership tags and PagerDuty destination secret
references. Customers will not need to compile code or build a container.

The initial scope is notification routing. Resource modification and automatic
alarm suppression are outside that scope.
