# Example applications

The [HTTP demonstration application](http-demo/README.md) provides a minimal,
stateless workload for local deployment, update, and recovery scenarios.

Its healthy v1/v2 images and deliberately unready v3 image are built separately.
Images remain local during initial development; registry publication and testing
on the Raspberry Pi are subsequent validation activities.

The demo is also used by the current broker lifecycle. Native builds and local
startup use `scripts/platform.sh`; the
[status report](../docs/status.md) distinguishes local evidence from target-host
and public-release acceptance.
