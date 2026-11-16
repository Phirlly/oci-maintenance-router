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

<p align="center">
  <a href="https://cloud.oracle.com/resourcemanager/stacks/create?zipUrl=https%3A%2F%2Fgithub.com%2FPhirlly%2Foci-maintenance-router%2Freleases%2Fdownload%2Fv0.0.3%2Foci-maintenance-router-v0.0.3.zip"><img src="https://oci-resourcemanager-plugin.plugins.oci.oraclecloud.com/latest/deploy-to-oracle-cloud.svg" alt="Deploy to Oracle Cloud"></a>
  <br>
  <a href="artifacts/oci-maintenance-router-v0.0.3/README.md">v0.0.3 package and checksum</a>
</p>

[Step-by-step deployment walkthrough](docs/deployment-walkthrough.md) — deploy in TEST and see the PagerDuty alert.

![Target architecture: OCI MySQL maintenance notices routed to PagerDuty using resource ownership tags (work in progress)](assets/oci-maintenance-router.gif)

[MIT License](LICENSE). An independent community solution integrating OCI and
PagerDuty. It is not developed, maintained, sponsored, or supported by Oracle.
