# oci-maintenance-router

An OCI maintenance-notification router intended to send relevant MySQL
maintenance notices to the PagerDuty service responsible for each affected
resource, using current ownership tags instead of a manually maintained list
of resource IDs.

Status: implementation in progress. Deployment-settings validation, notice
normalization with stable revision IDs, ownership-based routing decisions,
recipient-specific message drafts, delivery-state/recovery rules, and complete
notice records are implemented in Go. Source observations, resource assignments,
saved messages and outcomes share one conditional Object Storage write. Local TLS
tests cover concurrent updates and lost write responses. Scan coordination,
activation, secret retrieval and PagerDuty transport still need implementation
and testing in OCI. Customer scope, tags,
destinations and environment settings come from validated configuration. Unresolved
resources and messages exceeding application limits remain visible for review.
The local Function entrypoint performs a nonsending installation check only.
Prebuilt Function and Resource Manager ZIP packaging are implemented, with
dependency, license, executable and integrity checks. The Terraform stack and
native RM form have offline validation; deployment in OCI remains unverified.
This is not yet an operational router.

The intended installation is a **Deploy to OCI** button and a Resource Manager
form for resource scope, ownership tags and PagerDuty destination secret
references. Customers will not need to compile code or build a container.

The initial scope is notification routing. Resource modification and automatic
alarm suppression are outside that scope.

The current **PREVIEW** stack uses Terraform1.5.7 and OCI provider9.12.1. Its form
collects the deployment compartment, a stable environment name, MySQL regions
and compartments, ownership tag, named Vault secret references and routing map.
For example, destination `payments` references a regional Vault secret; owner
values `payments-api` and `billing` can both map to that destination. Secret
contents are never entered in the form or retrieved by this installation check.
Routes are case-sensitive. Owner values must already use Unicode NFC because
Terraform normalizes strings; ASCII values are unaffected. Decomposed and
composed spellings cannot be represented as distinct native-form keys.

The deploying user needs Resource Manager access and authorization to create
Functions, topics, a private versioned bucket and configuration objects in the
chosen compartment, plus the exact Function's dynamic group and IAM policy.
Creating the default dedicated network also needs VCN, subnet, route, security
list and service-gateway permissions. Existing-network mode instead selects a
VCN and private regional subnet with DNS and HTTPS access to regional Object
Storage. Regional subscription/runtime reads and Function read/invoke access
are required. Vault secret-list access is used by the picker; the stack never
reads secret contents. The selected region must offer the package's exact
approved runtime; an unavailable version fails rather than selecting another.

For an approved isolated installation, use the complete prebuilt ZIP as the RM
stack configuration, select Terraform1.5.x, fill in the form, review Plan and
run Apply. The stack creates its resources and runs a nonsending installation
check. A successful result means configuration and storage access were verified;
`sending_enabled` remains false. A public package URL and working **Deploy to
OCI** button are still pending validation and publication.

If the check fails, inspect its safe result in the job and verify IAM/network
access. IAM propagation can delay access; rerun Apply after it is ready. Every
Apply performs a fresh check. Save changed form values and Apply to update
configuration. Previous object versions are retained; the state bucket is
protected from normal Terraform destruction. Keep the environment name stable.
Do not remove the bucket's protection or delete retained versions as routine
cleanup. Upgrade, rollback and teardown still require isolated OCI testing.
