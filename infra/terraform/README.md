# Infrastructure

Terraform provisioning is not implemented. This directory is a placeholder for a
possible rented-server/DNS boundary; the decision remains open. Current local
installation uses `scripts/platform.sh` and k3d, not Terraform.

The [implementation plan](../../docs/milestones.md) and
[status report](../../docs/status.md) distinguish production installation and
release requirements from the existing local workflow. Any future implementation
must document initialization, provisioning and teardown and exclude credentials
and Terraform state from source control.
