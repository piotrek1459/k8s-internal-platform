# Example applications

The [HTTP demonstration application](http-demo/README.md) provides a minimal,
stateless workload for local deployment, update, and recovery scenarios.

Its healthy v1/v2 images and deliberately unready v3 image are built separately.
Development images are published only to the selected local registry. The manual
release bundles include native ARM64/AMD64 OCI archives for all three versions;
public container-registry publishing and native Raspberry Pi acceptance remain
separate requirements.

The demo is also used by the current broker lifecycle. Native builds and local
startup use `scripts/platform.sh`; the
[status report](../docs/status.md) distinguishes local evidence from target-host
and public-release acceptance.
