package helm

import (
	"context"
	"encoding/json"
	"helm.sh/helm/v3/pkg/action"
	"io"
	"k8s-internal-platform/broker/internal/platform"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/util/yaml"
	"strings"
	"testing"
)

func TestSnapshotVerifiesReleaseIdentityAndRejectsOtherRevision(t *testing.T) {
	a, err := NewAdapter("platform-apps", memoryActionConfig(t))
	if err != nil {
		t.Fatal(err)
	}
	a.WithManifestVerifier(func(context.Context, string) error { return nil })
	rev := frozenRevision("app_01", baseSpec())
	frozen, err := a.PrepareExecution(rev.AppID, rev)
	if err != nil {
		t.Fatal(err)
	}
	rev.Frozen = &frozen
	if frozen.ChartArtifactDigest == "" || frozen.ValuesHash == "" {
		t.Fatal("incomplete execution snapshot")
	}
	if _, err := a.InstallOrUpgrade(context.Background(), rev.AppID, rev); err != nil {
		t.Fatal(err)
	}
	if err := a.VerifyTarget(context.Background(), rev.AppID, rev); err != nil {
		t.Fatalf("verify applied target: %v", err)
	}
	rev.ID = "unapplied-revision"
	if err := a.VerifyTarget(context.Background(), rev.AppID, rev); err == nil {
		t.Fatal("old deployed release accepted as target")
	}
}

func TestSnapshotRejectsUnavailableChartAndChangedValues(t *testing.T) {
	a, _ := NewAdapter("platform-apps", memoryActionConfig(t))
	rev := frozenRevision("app_01", baseSpec())
	frozen, _ := a.PrepareExecution(rev.AppID, rev)
	rev.Frozen = &frozen
	rev.Spec.Replicas = 2
	if _, err := a.PrepareExecution(rev.AppID, rev); err == nil {
		t.Fatal("changed values accepted")
	}
	rev.Spec.Replicas = 1
	rev.Frozen.ValuesHash = ""
	rev.Frozen.ChartArtifactDigest = "sha256:unavailable"
	if _, err := a.InstallOrUpgrade(context.Background(), rev.AppID, rev); err == nil {
		t.Fatal("substituted chart accepted")
	}
}

func TestStartupAndReadinessAreIndependent(t *testing.T) {
	probes := probeValues(platform.Health{ReadinessPath: "/ready"})
	if probes["startup"].(map[string]any)["type"] != "tcp" {
		t.Fatal("readiness reused for startup")
	}
	probes = probeValues(platform.Health{ReadinessPath: "/ready", StartupPath: "/started"})
	if probes["startup"].(map[string]any)["path"] != "/started" || probes["readiness"].(map[string]any)["path"] != "/ready" {
		t.Fatal("independent probe paths lost")
	}
}

func TestRenderedApplicationHasTemporaryStorageWithReadOnlyRoot(t *testing.T) {
	configuration := memoryActionConfig(t)
	adapter, err := NewAdapter("platform-apps", configuration)
	if err != nil {
		t.Fatal(err)
	}
	revision := frozenRevision("app_tmp", baseSpec())
	if _, err := adapter.InstallOrUpgrade(context.Background(), revision.AppID, revision); err != nil {
		t.Fatal(err)
	}
	cfg, err := configuration("platform-apps")
	if err != nil {
		t.Fatal(err)
	}
	release, err := action.NewGet(cfg).Run(releaseNameFor(revision.AppID))
	if err != nil {
		t.Fatal(err)
	}
	decoder := yaml.NewYAMLOrJSONDecoder(strings.NewReader(release.Manifest), 4096)
	for {
		var raw json.RawMessage
		if err := decoder.Decode(&raw); err == io.EOF {
			break
		} else if err != nil {
			t.Fatal(err)
		}
		var deployment appsv1.Deployment
		if err := json.Unmarshal(raw, &deployment); err != nil {
			t.Fatal(err)
		}
		if deployment.Kind != "Deployment" {
			continue
		}
		pod := deployment.Spec.Template.Spec
		if pod.SecurityContext.FSGroup == nil || *pod.SecurityContext.FSGroup != 2000 {
			t.Fatal("temporary directory must be writable by non-root image users")
		}
		if len(pod.Volumes) != 1 || pod.Volumes[0].EmptyDir == nil || pod.Volumes[0].EmptyDir.SizeLimit == nil || pod.Volumes[0].EmptyDir.SizeLimit.String() != "64Mi" {
			t.Fatal("temporary storage must have its own explicit size limit")
		}
		container := pod.Containers[0]
		if container.SecurityContext.ReadOnlyRootFilesystem == nil || !*container.SecurityContext.ReadOnlyRootFilesystem || container.SecurityContext.AllowPrivilegeEscalation == nil || *container.SecurityContext.AllowPrivilegeEscalation {
			t.Fatal("temporary storage must preserve container hardening")
		}
		if len(container.VolumeMounts) != 1 || container.VolumeMounts[0].MountPath != "/tmp" || container.VolumeMounts[0].Name != pod.Volumes[0].Name {
			t.Fatal("temporary storage is not mounted at /tmp")
		}
		return
	}
	t.Fatal("rendered Deployment is missing")
}
