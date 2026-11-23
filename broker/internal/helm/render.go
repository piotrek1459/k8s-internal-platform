package helm

import (
	"sort"
	"strings"

	"helm.sh/helm/v3/pkg/chartutil"
	"helm.sh/helm/v3/pkg/engine"

	"k8s-internal-platform/broker/internal/platform"
)

// RenderExecution reconstructs persisted intent with the matching embedded
// chart. It performs no Helm/Kubernetes reads or writes.
func (a *Adapter) RenderExecution(appID string, rev platform.Revision) (string, error) {
	if _, err := a.PrepareExecution(appID, rev); err != nil {
		return "", err
	}
	values, err := BuildValues(appID, rev)
	if err != nil {
		return "", err
	}
	renderValues, err := chartutil.ToRenderValues(a.chart, values, chartutil.ReleaseOptions{Name: releaseNameFor(appID), Namespace: a.namespace, IsUpgrade: true, Revision: 1}, nil)
	if err != nil {
		return "", platform.Errorf(platform.CodeConflict, "the recorded execution cannot be rendered")
	}
	rendered, err := engine.Render(a.chart, renderValues)
	if err != nil {
		return "", platform.Errorf(platform.CodeConflict, "the recorded execution cannot be rendered")
	}
	names := make([]string, 0, len(rendered))
	for name := range rendered {
		if !strings.HasSuffix(name, "NOTES.txt") && strings.TrimSpace(rendered[name]) != "" {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	var body strings.Builder
	for _, name := range names {
		body.WriteString("---\n")
		body.WriteString(rendered[name])
		body.WriteByte('\n')
	}
	return body.String(), nil
}
