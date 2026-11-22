package helm

import (
	"context"
	"k8s-internal-platform/broker/internal/platform"
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
