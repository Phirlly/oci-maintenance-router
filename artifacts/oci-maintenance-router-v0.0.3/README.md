# OCI Maintenance Router v0.0.3

[Download v0.0.3](oci-maintenance-router-v0.0.3.zip) · [SHA-256](oci-maintenance-router-v0.0.3.zip.sha256) · [Deploy to Oracle Cloud](https://cloud.oracle.com/resourcemanager/stacks/create?zipUrl=https%3A%2F%2Fgithub.com%2FPhirlly%2Foci-maintenance-router%2Freleases%2Fdownload%2Fv0.0.3%2Foci-maintenance-router-v0.0.3.zip)

Includes Terraform and the prebuilt Function for HeatWave MySQL maintenance routing by ownership tags. **Development version; not ready for production use.**

Compared with [v0.0.2](../oci-maintenance-router-v0.0.2/README.md), this version adds a five-minute wait after IAM changes during Apply and improves startup diagnostics. One fresh TEST deployment and its matching PagerDuty test alert were verified. Deployment time varies; five minutes is not an IAM readiness guarantee.

Start in TEST with a nonproduction PagerDuty service. Existing installations must run a new test for this package and confirm receipt before LIVE activation because the Function archive identity changed.

Archive size: 4,537,748 bytes. Retain this version unchanged; later packages use new versioned filenames.
