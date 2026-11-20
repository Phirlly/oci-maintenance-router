locals {
  # Only IAM changes renew the provisioning wait, not each Apply or archive update.
  startup_iam_generation = sha256(jsonencode({
    group = {
      id            = oci_identity_dynamic_group.router.id
      matching_rule = oci_identity_dynamic_group.router.matching_rule
    }
    policies = concat(
      [for policy in [oci_identity_policy.router, oci_identity_policy.announcements] : { id = policy.id, statements = policy.statements }],
      [for policy in oci_identity_policy.diagnostic : { id = policy.id, statements = policy.statements }],
      [for policy in oci_identity_policy.operator : { id = policy.id, statements = policy.statements }],
      [for policy in oci_identity_policy.setup : { id = policy.id, statements = policy.statements }],
      [for policy in oci_identity_policy.live : { id = policy.id, statements = policy.statements }],
    )
  }))
}

resource "time_sleep" "iam_propagation" {
  # Provisional fresh-install allowance, not proof that IAM is ready.
  # The deployment check still verifies actual access and fails closed.
  create_duration = "5m"
  triggers = {
    iam_generation = local.startup_iam_generation
  }
  depends_on = [oci_identity_policy.router, oci_identity_policy.announcements, oci_identity_policy.diagnostic, oci_identity_policy.operator, oci_identity_policy.setup, oci_identity_policy.live]
}
