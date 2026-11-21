# OCI Maintenance Router

The goal: route all OCI announcements affecting the resource types and ownership
tags you select to the right PagerDuty team. Deploy and configure through OCI
Resource Manager.

- **Supported today:** HeatWave MySQL scheduled maintenance, routed using defined
  or freeform resource ownership tags.
- **In progress:** Selectable resource types, starting with Compute, broader
  announcement coverage and end-to-end validation. These are not yet available
  in the published package.
- **Planned:** Resource-name matching and optional routing for notices without
  resource IDs.

**Under development — not ready for production use.**

<p align="center">
  <a href="https://cloud.oracle.com/resourcemanager/stacks/create?zipUrl=https%3A%2F%2Fgithub.com%2FPhirlly%2Foci-maintenance-router%2Freleases%2Fdownload%2Fv0.0.3%2Foci-maintenance-router-v0.0.3.zip"><img src="https://oci-resourcemanager-plugin.plugins.oci.oraclecloud.com/latest/deploy-to-oracle-cloud.svg" alt="Deploy to Oracle Cloud"></a>
  <br>
  <a href="artifacts/oci-maintenance-router-v0.0.3/README.md">v0.0.3 package and checksum</a>
</p>

[Step-by-step deployment walkthrough](docs/deployment-walkthrough.md) — deploy in TEST and see the PagerDuty alert.

![MySQL routing architecture: maintenance notices routed to PagerDuty using resource ownership tags](assets/oci-maintenance-router.gif)

[MIT License](LICENSE). An independent community solution integrating OCI and
PagerDuty. It is not developed, maintained, sponsored, or supported by Oracle.
