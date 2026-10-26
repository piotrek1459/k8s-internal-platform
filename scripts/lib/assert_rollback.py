"""Validate recovery independently of the HTTP response from surviving old Pods."""

import json
import sys
from pathlib import Path


def validate_rollback(failed_release, release, deployment, replica_sets, pods):
    assert release["version"] > failed_release["version"], "No new Helm revision"
    assert release["info"]["status"] == "deployed", "Helm release is not deployed"

    spec = deployment["spec"]
    status = deployment.get("status", {})
    assert spec["template"]["spec"]["containers"][0]["image"] == "platform-demo:v2", \
        "Deployment still targets the failed image"
    assert status.get("observedGeneration", 0) >= deployment["metadata"]["generation"], \
        "Deployment controller has not observed the restored specification"
    desired = spec["replicas"]
    assert desired > 0, "The restored workload must serve traffic"
    for field in ("replicas", "updatedReplicas", "readyReplicas", "availableReplicas"):
        assert status.get(field, 0) == desired, f"Deployment {field} has not converged"

    for replica_set in replica_sets["items"]:
        image = replica_set["spec"]["template"]["spec"]["containers"][0]["image"]
        if image == "platform-demo:v3":
            assert replica_set["spec"]["replicas"] == 0, "Failed ReplicaSet is still desired"
    # Old ReplicaSet objects may remain for rollout history; failed Pods may not.
    assert not any(
        container["image"] == "platform-demo:v3"
        for pod in pods["items"]
        for container in pod["spec"]["containers"]
    ), "Failed v3 Pods still exist"


def main():
    results = Path(sys.argv[1])
    filenames = (
        "failed-release.json", "rollback-release.json", "rollback-deployment.json",
        "rollback-replicasets.json", "rollback-pods.json",
    )
    try:
        validate_rollback(*(json.loads((results / name).read_text()) for name in filenames))
    except AssertionError as error:
        print(f"Rollback has not converged: {error}", file=sys.stderr)
        return 1
    print("Verified rollback: new deployed Helm revision, ready v2 Deployment, no v3 Pods.")
    return 0


if __name__ == "__main__":
    sys.exit(main())
