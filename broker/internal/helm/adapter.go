// Package helm renders and applies the first-party application chart through the
// in-process Helm v3 SDK (not the CLI). It maps a frozen application revision to
// the chart's documented values (deploy/helm/application/values.schema.json),
// relying on Helm's own schema validation, and performs a bounded install or
// upgrade without automatic rollback — recovery is the broker's responsibility.
package helm

import (
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"time"

	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/storage/driver"

	"k8s-internal-platform/broker/internal/platform"
)

// chartFS embeds the first-party application chart so the broker ships as a
// single artifact. It is a copy of deploy/helm/application; scripts/check-chart.sh
// verifies the two stay in sync.
//
//go:embed all:chart
var chartFS embed.FS

// installTimeout bounds a single install or upgrade. It is aligned with the
// chart's progressDeadlineSeconds (180s) plus headroom for image pulls.
const installTimeout = 5 * time.Minute

// Adapter installs and upgrades the application chart in a target namespace.
type Adapter struct {
	chart     *chart.Chart
	namespace string
	// actionConfigFor builds a Helm action configuration bound to a release
	// namespace. It is a field so tests can inject an in-memory configuration.
	actionConfigFor func(namespace string) (*action.Configuration, error)
}

var _ platform.HelmInstaller = (*Adapter)(nil)

// NewAdapter loads the embedded chart and binds the adapter to a namespace and a
// Helm action-configuration factory. In production, pass a factory built from the
// in-cluster config; tests pass a memory-backed factory.
func NewAdapter(namespace string, actionConfigFor func(string) (*action.Configuration, error)) (*Adapter, error) {
	ch, err := loadEmbeddedChart()
	if err != nil {
		return nil, err
	}
	return &Adapter{chart: ch, namespace: namespace, actionConfigFor: actionConfigFor}, nil
}

// ChartVersion reports the loaded chart's version, recorded in frozen revisions.
func (a *Adapter) ChartVersion() string { return a.chart.Metadata.Version }

// InstallOrUpgrade renders the chart from the revision's frozen spec and installs
// or upgrades the release for appID. It requires a frozen revision, because the
// values derive from the immutable execution spec.
func (a *Adapter) InstallOrUpgrade(ctx context.Context, appID string, rev platform.Revision) (platform.ReleaseStatus, error) {
	if rev.Frozen == nil {
		return platform.ReleaseStatus{}, platform.Errorf(platform.CodeInternal,
			"revision %s is not frozen; cannot install", rev.ID)
	}
	values, err := BuildValues(appID, rev)
	if err != nil {
		return platform.ReleaseStatus{}, err
	}

	cfg, err := a.actionConfigFor(a.namespace)
	if err != nil {
		return platform.ReleaseStatus{}, platform.Errorf(platform.CodeDependencyUnavailable,
			"init helm: %v", err)
	}
	releaseName := releaseNameFor(appID)

	exists, err := releaseExists(cfg, releaseName)
	if err != nil {
		return platform.ReleaseStatus{}, err
	}

	if exists {
		return a.upgrade(ctx, cfg, releaseName, values)
	}
	return a.install(ctx, cfg, releaseName, values)
}

func (a *Adapter) install(ctx context.Context, cfg *action.Configuration, name string, values map[string]any) (platform.ReleaseStatus, error) {
	client := action.NewInstall(cfg)
	client.ReleaseName = name
	client.Namespace = a.namespace
	client.Timeout = installTimeout
	client.Wait = true
	rel, err := client.RunWithContext(ctx, a.chart, values)
	if err != nil {
		return platform.ReleaseStatus{}, classifyHelmError(err)
	}
	return platform.ReleaseStatus{Revision: rel.Version, Status: rel.Info.Status.String()}, nil
}

func (a *Adapter) upgrade(ctx context.Context, cfg *action.Configuration, name string, values map[string]any) (platform.ReleaseStatus, error) {
	client := action.NewUpgrade(cfg)
	client.Namespace = a.namespace
	client.Timeout = installTimeout
	client.Wait = true
	// No automatic rollback: the broker owns recovery and must observe the failure
	// to reach Failed/NeedsAttention per the execution rules.
	client.Atomic = false
	rel, err := client.RunWithContext(ctx, name, a.chart, values)
	if err != nil {
		return platform.ReleaseStatus{}, classifyHelmError(err)
	}
	return platform.ReleaseStatus{Revision: rel.Version, Status: rel.Info.Status.String()}, nil
}

// Uninstall removes the release's resources. A missing release is not an error so
// a retried deletion converges.
func (a *Adapter) Uninstall(ctx context.Context, appID string) error {
	cfg, err := a.actionConfigFor(a.namespace)
	if err != nil {
		return platform.Errorf(platform.CodeDependencyUnavailable, "init helm: %v", err)
	}
	name := releaseNameFor(appID)
	exists, err := releaseExists(cfg, name)
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}
	client := action.NewUninstall(cfg)
	client.Timeout = installTimeout
	client.Wait = true
	if _, err := client.Run(name); err != nil {
		return classifyHelmError(err)
	}
	return nil
}

// Status reports the current release status without mutating it.
func (a *Adapter) Status(ctx context.Context, appID string) (platform.ReleaseStatus, bool, error) {
	cfg, err := a.actionConfigFor(a.namespace)
	if err != nil {
		return platform.ReleaseStatus{}, false, platform.Errorf(platform.CodeDependencyUnavailable, "init helm: %v", err)
	}
	rel, err := action.NewGet(cfg).Run(releaseNameFor(appID))
	if err != nil {
		if err == driver.ErrReleaseNotFound {
			return platform.ReleaseStatus{}, false, nil
		}
		return platform.ReleaseStatus{}, false, classifyHelmError(err)
	}
	return platform.ReleaseStatus{Revision: rel.Version, Status: rel.Info.Status.String()}, true, nil
}

func releaseExists(cfg *action.Configuration, name string) (bool, error) {
	_, err := action.NewGet(cfg).Run(name)
	if err == nil {
		return true, nil
	}
	if err == driver.ErrReleaseNotFound {
		return false, nil
	}
	return false, classifyHelmError(err)
}

// loadEmbeddedChart reads the embedded chart directory into Helm's loader. The
// chart files are converted to loader.BufferedFile entries with paths relative to
// the chart root, as the loader expects.
func loadEmbeddedChart() (*chart.Chart, error) {
	var files []*loader.BufferedFile
	err := fs.WalkDir(chartFS, "chart", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		data, err := chartFS.ReadFile(path)
		if err != nil {
			return err
		}
		// Strip the leading "chart/" so the loader sees chart-root-relative paths.
		rel := path[len("chart/"):]
		files = append(files, &loader.BufferedFile{Name: rel, Data: data})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk embedded chart: %w", err)
	}
	ch, err := loader.LoadFiles(files)
	if err != nil {
		return nil, fmt.Errorf("load embedded chart: %w", err)
	}
	return ch, nil
}

func releaseNameFor(appID string) string {
	// Release names must be DNS-1123; opaque app IDs may contain underscores, so
	// sanitize to keep Helm and Kubernetes happy while the app ID remains a label.
	return "app-" + sanitize(appID)
}

func sanitize(id string) string {
	out := make([]rune, 0, len(id))
	for _, r := range id {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-':
			out = append(out, r)
		case r >= 'A' && r <= 'Z':
			out = append(out, r+('a'-'A'))
		default:
			out = append(out, '-')
		}
	}
	return string(out)
}

// classifyHelmError maps Helm errors to domain errors. A rollout or wait failure
// is reported so the worker records it; it is not retried blindly.
func classifyHelmError(err error) error {
	return platform.Errorf(platform.CodeDependencyUnavailable, "helm operation failed: %v", err)
}

// valuesHash returns a stable hash of the rendered values, recorded on the frozen
// revision so recovery can relate cluster changes to persisted intent.
func valuesHash(values map[string]any) string {
	canonical, _ := json.Marshal(values)
	sum := sha256.Sum256(canonical)
	return hex.EncodeToString(sum[:])
}
