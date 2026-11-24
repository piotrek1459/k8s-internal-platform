package cluster

import (
	"strings"
	"testing"

	"helm.sh/helm/v3/pkg/release"
	"helm.sh/helm/v3/pkg/storage/driver"
)

func TestHelmFailureStorageExcludesUntrustedErrorBodyWithoutChangingIdentity(t *testing.T) {
	raw := driver.NewMemory()
	raw.SetNamespace(ns)
	safe := secretSafeReleaseDriver{raw}
	value := &release.Release{Name: "app-app-1", Namespace: ns, Version: 1, Info: &release.Info{Status: release.StatusDeployed, Description: "platform revision rev_original"}, Config: map[string]any{"reference": "secret-reference-only"}}
	if err := safe.Create("sh.helm.release.v1.app-app-1.v1", value); err != nil {
		t.Fatal(err)
	}
	stored, err := safe.Get("sh.helm.release.v1.app-app-1.v1")
	if err != nil || stored.Info.Description != "platform revision rev_original" {
		t.Fatal("successful revision identity changed")
	}
	marker := "controlled-sensitive-admission-body"
	value.Info = &release.Info{Status: release.StatusFailed, Description: marker}
	if err := safe.Update("sh.helm.release.v1.app-app-1.v1", value); err != nil {
		t.Fatal(err)
	}
	stored, err = safe.Get("sh.helm.release.v1.app-app-1.v1")
	if err != nil || strings.Contains(stored.Info.Description, marker) || stored.Name != value.Name || stored.Version != value.Version || stored.Config["reference"] != value.Config["reference"] {
		t.Fatal("failed storage exposed a response body or lost release identity")
	}
	if value.Info.Description != marker {
		t.Fatal("storage sanitizer mutated the SDK's in-memory release")
	}
}
