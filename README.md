# OCI Maintenance Router

Route OCI maintenance announcements to the right PagerDuty team, with deployment
and configuration through OCI Resource Manager. Support will expand to additional
OCI services over time.

- **Supported today:** HeatWave MySQL scheduled maintenance, routed using defined
  or freeform resource ownership tags.
- **In progress:** Deployment usability improvements and remaining end-to-end
  validation.
- **Planned:** Resource-name matching and additional OCI services.

**Under development — not ready for production use.**

[Download v0.0.1 demo RM ZIP](artifacts/oci-maintenance-router-v0.0.1/oci-maintenance-router-v0.0.1.zip) · [Package details and checksum](artifacts/oci-maintenance-router-v0.0.1/README.md)

**Deploy to OCI — coming soon.** The button will be available once the deployment
package is validated and published.

![Target architecture: OCI MySQL maintenance notices routed to PagerDuty using resource ownership tags (work in progress)](assets/oci-maintenance-router.gif)
