package helm

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"

	"k8s-internal-platform/broker/internal/platform"
)

// PrepareExecution records the embedded chart and canonical values identity.
// Old chart artifacts are deliberately refused rather than silently substituted.
func (a *Adapter) PrepareExecution(appID string, rev platform.Revision) (platform.FrozenSpec, error) {
	if rev.Frozen == nil {
		return platform.FrozenSpec{}, platform.Errorf(platform.CodeInternal, "execution requires a pinned image")
	}
	frozen := *rev.Frozen
	digest := chartDigest(a.chart)
	if frozen.ChartArtifactDigest != "" && frozen.ChartArtifactDigest != digest {
		return frozen, platform.Errorf(platform.CodeUnsupportedSpec, "the recorded chart artifact is unavailable; rollback across chart changes is unsupported")
	}
	hash, err := ValuesHash(appID, rev)
	if err != nil {
		return frozen, err
	}
	if frozen.ValuesHash != "" && frozen.ValuesHash != hash {
		return frozen, platform.Errorf(platform.CodeConflict, "execution values differ from the frozen snapshot")
	}
	frozen.ChartVersion = a.ChartVersion()
	frozen.ChartArtifactDigest = digest
	frozen.ValuesHash = hash
	return frozen, nil
}

func chartDigest(ch *chart.Chart) string {
	// Chart's JSON contains metadata, templates, schema, defaults and files, and
	// survives Helm release storage. This first-party chart has no dependencies.
	body, _ := json.Marshal(ch)
	sum := sha256.Sum256(body)
	return "sha256:" + hex.EncodeToString(sum[:])
}

// WithManifestVerifier supplies live, namespaced resource verification.
func (a *Adapter) WithManifestVerifier(verify func(context.Context, string) error) *Adapter {
	a.verifyManifest = verify
	return a
}

// VerifyTarget requires release identity, snapshot identity and healthy live
// resources. An unrelated old deployed release never establishes target success.
func (a *Adapter) VerifyTarget(ctx context.Context, appID string, rev platform.Revision) error {
	if rev.Frozen == nil || rev.Frozen.ChartArtifactDigest == "" || rev.Frozen.ValuesHash == "" {
		return platform.Errorf(platform.CodeConflict, "the revision has no complete execution snapshot")
	}
	cfg, err := a.actionConfigFor(a.namespace)
	if err != nil {
		return err
	}
	rel, err := action.NewGet(cfg).Run(releaseNameFor(appID))
	if err != nil {
		return err
	}
	if rel.Info.Status.String() != "deployed" || rel.Info.Description != "platform revision "+rev.ID || chartDigest(rel.Chart) != rev.Frozen.ChartArtifactDigest || valuesHash(rel.Config) != rev.Frozen.ValuesHash {
		return platform.Errorf(platform.CodeConflict, "the deployed release does not match the target revision")
	}
	if a.verifyManifest == nil {
		return platform.Errorf(platform.CodeDependencyUnavailable, "live release verification is unavailable")
	}
	return a.verifyManifest(ctx, rel.Manifest)
}
