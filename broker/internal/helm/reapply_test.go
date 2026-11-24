package helm

import (
	"context"
	"errors"
	"strings"
	"testing"

	"helm.sh/helm/v3/pkg/release"
)

func TestMutationGuardStopsApplyAndUninstallBeforeHelmMutation(t *testing.T) {
	factory := memoryActionConfig(t)
	adapter, err := NewAdapter("platform-apps", factory)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := factory("platform-apps")
	if err != nil {
		t.Fatal(err)
	}
	revision := frozenRevision("app_1", baseSpec())
	frozen, err := adapter.PrepareExecution("app_1", revision)
	if err != nil {
		t.Fatal(err)
	}
	revision.Frozen = &frozen
	target, err := adapter.RenderExecution("app_1", revision)
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.Releases.Create(&release.Release{Name: "app-app-1", Namespace: "platform-apps", Version: 1, Info: &release.Info{Status: release.StatusDeployed}, Manifest: target}); err != nil {
		t.Fatal(err)
	}
	blocked := errors.New("ownership conflict")
	calls := 0
	adapter.WithMutationGuard(func(_ context.Context, id, body string) error {
		calls++
		if id != "app_1" || !strings.Contains(body, "kind: Service") {
			t.Fatal("wrong mutation guard target")
		}
		return blocked
	})
	if _, err := adapter.InstallOrUpgrade(context.Background(), "app_1", revision); !errors.Is(err, blocked) {
		t.Fatal("upgrade bypassed ownership guard")
	}
	if err := adapter.Uninstall(context.Background(), "app_1"); !errors.Is(err, blocked) {
		t.Fatal("uninstall bypassed ownership guard")
	}
	latest, err := cfg.Releases.Last("app-app-1")
	if err != nil || latest.Version != 1 || latest.Info.Status != release.StatusDeployed || calls != 2 {
		t.Fatal("guarded operation changed Helm state")
	}
}

func TestReapplyPreparationRefusesPendingReleaseBeforeAnyMutation(t *testing.T) {
	for _, status := range []release.Status{release.StatusDeployed, release.StatusFailed, release.StatusPendingUpgrade, release.StatusPendingInstall, release.StatusPendingRollback} {
		t.Run(string(status), func(t *testing.T) {
			factory := memoryActionConfig(t)
			cfg, err := factory("platform-apps")
			if err != nil {
				t.Fatal(err)
			}
			if err := cfg.Releases.Create(&release.Release{Name: "app-app-1", Namespace: "platform-apps", Version: 1, Info: &release.Info{Status: status}}); err != nil {
				t.Fatal(err)
			}
			adapter, err := NewAdapter("platform-apps", factory)
			if err != nil {
				t.Fatal(err)
			}
			revision := frozenRevision("app_1", baseSpec())
			frozen, err := adapter.PrepareExecution("app_1", revision)
			if err != nil {
				t.Fatal(err)
			}
			revision.Frozen = &frozen
			calls := 0
			adapter.WithReapplyPreparer(func(_ context.Context, id, manifest string) error {
				calls++
				if id != "app_1" || !strings.Contains(manifest, "kind: Service") {
					t.Fatal("wrong preparation target")
				}
				return nil
			})
			err = adapter.PrepareReapply(context.Background(), "app_1", revision)
			allowed := status == release.StatusDeployed || status == release.StatusFailed
			if allowed && (err != nil || calls != 1) {
				t.Fatalf("supported preparation: %v calls=%d", err, calls)
			}
			if !allowed && (err == nil || calls != 0) {
				t.Fatal("pending release permitted pre-Helm mutation")
			}
		})
	}
}

func TestRestorationPreparationRefusesPendingReleaseBeforeRepair(t *testing.T) {
	for _, status := range []release.Status{release.StatusDeployed, release.StatusFailed, release.StatusPendingUpgrade, release.StatusPendingInstall, release.StatusPendingRollback} {
		t.Run(string(status), func(t *testing.T) {
			factory := memoryActionConfig(t)
			cfg, err := factory("platform-apps")
			if err != nil {
				t.Fatal(err)
			}
			if err := cfg.Releases.Create(&release.Release{Name: "app-app-1", Namespace: "platform-apps", Version: 1, Info: &release.Info{Status: status}}); err != nil {
				t.Fatal(err)
			}
			adapter, err := NewAdapter("platform-apps", factory)
			if err != nil {
				t.Fatal(err)
			}
			r := frozenRevision("app_1", baseSpec())
			f, err := adapter.PrepareExecution("app_1", r)
			if err != nil {
				t.Fatal(err)
			}
			r.Frozen = &f
			calls := 0
			adapter.WithRestorationPreparer(func(context.Context, string, string) error { calls++; return nil })
			err = adapter.PrepareRestoration(context.Background(), "app_1", r)
			allowed := status == release.StatusDeployed || status == release.StatusFailed
			if allowed && (err != nil || calls != 1) {
				t.Fatalf("supported restoration: %v calls=%d", err, calls)
			}
			if !allowed && (err == nil || calls != 0) {
				t.Fatal("pending release permitted resource repair")
			}
		})
	}
}
