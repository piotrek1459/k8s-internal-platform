package helm

import (
	"strconv"

	"k8s-internal-platform/broker/internal/platform"
)

// BuildValues maps a frozen application revision to the application chart's
// values, matching deploy/helm/application/values.schema.json. Helm validates the
// result against that schema at install time, so an unsupported value surfaces as
// a chart error rather than a malformed manifest. The image is always pinned by
// the frozen digest (never a mutable tag), satisfying the schema's requirement
// that exactly one of image.tag/image.digest is set.
func BuildValues(appID string, rev platform.Revision) (map[string]any, error) {
	if rev.Frozen == nil || rev.Frozen.ImageDigest == "" {
		return nil, platform.Errorf(platform.CodeInternal, "revision %s has no frozen image digest", rev.ID)
	}
	spec := rev.Spec

	repository, digest, err := splitImageDigest(rev.Frozen.ImageDigest)
	if err != nil {
		return nil, err
	}

	values := map[string]any{
		"appId": appID,
		"image": map[string]any{
			"repository": repository,
			"tag":        "",
			"digest":     digest,
			"pullPolicy": "IfNotPresent",
		},
		"replicas":         spec.Replicas,
		"containerPort":    spec.ContainerPort,
		"env":              stringMap(spec.Environment),
		"secretRefs":       secretRefValues(rev.Frozen.SecretVersions, spec),
		"imagePullSecrets": pullSecretValues(spec.RegistryCredentialID),
		"resources":        resourceValues(spec.Resources),
		"probes":           probeValues(spec.Health),
		"ingress":          ingressValues(spec.Exposure),
	}
	return values, nil
}

// ValuesHash returns the stable hash of the values built for a revision, for
// recording on the frozen spec.
func ValuesHash(appID string, rev platform.Revision) (string, error) {
	values, err := BuildValues(appID, rev)
	if err != nil {
		return "", err
	}
	return valuesHash(values), nil
}

// splitImageDigest splits "repo@sha256:..." into repository and digest. The
// frozen digest is always in this pinned form.
func splitImageDigest(ref string) (repository, digest string, err error) {
	for i := 0; i < len(ref); i++ {
		if ref[i] == '@' {
			return ref[:i], ref[i+1:], nil
		}
	}
	return "", "", platform.Errorf(platform.CodeInternal, "frozen image %q is not digest-pinned", ref)
}

func stringMap(m map[string]string) map[string]any {
	out := map[string]any{}
	for k, v := range m {
		out[k] = v
	}
	return out
}

// secretRefValues maps the revision's secret references to the chart's
// {name, secretName, key} form. The chart consumes an existing Kubernetes Secret;
// the broker created the immutable versioned Secret whose name is recorded on the
// frozen revision's SecretVersions.
func secretRefValues(frozen []platform.SecretRef, spec platform.RevisionSpec) []any {
	// Prefer the frozen references (which carry the resolved version); fall back to
	// the spec's references when none were frozen (no secrets in play).
	refs := frozen
	if len(refs) == 0 {
		refs = spec.SecretRefs
	}
	out := make([]any, 0, len(refs))
	for _, r := range refs {
		out = append(out, map[string]any{
			"name":       r.EnvironmentVariable,
			"secretName": secretObjectName(r),
			"key":        secretKeyName(r),
		})
	}
	return out
}

// secretObjectName derives the Kubernetes Secret name the chart should reference.
// It mirrors cluster.secretName so the chart binds to the Secret the broker
// created for that secret ID and version.
func secretObjectName(r platform.SecretRef) string {
	return "app-secret-" + sanitize(r.SecretID) + "-v" + strconv.Itoa(r.Version)
}

// secretKeyName is the key within the Secret. The platform stores a single
// "value" key per environment-mapped secret reference.
func secretKeyName(platform.SecretRef) string { return "value" }

func pullSecretValues(credentialID string) []any {
	if credentialID == "" {
		return []any{}
	}
	return []any{map[string]any{"name": "registry-" + sanitize(credentialID)}}
}

func resourceValues(r platform.ResourceRequirements) map[string]any {
	// Defaults match the chart's provisional preset when the request omits values,
	// so Helm's schema (which requires requests/limits) is always satisfied.
	req := map[string]any{"cpu": orDefault(r.CPURequest, "25m"), "memory": orDefault(r.MemoryRequest, "32Mi")}
	lim := map[string]any{"cpu": orDefault(r.CPULimit, "250m"), "memory": orDefault(r.MemoryLimit, "128Mi")}
	return map[string]any{"requests": req, "limits": lim}
}

// probeValues maps the requested readiness to the chart's probe block. When a
// readiness HTTP path is given it is used; otherwise the documented TCP fallback
// applies. Startup uses HTTP /healthz when readiness is HTTP, else TCP.
func probeValues(h platform.Health) map[string]any {
	readiness := httpProbe(h.ReadinessPath, "/readyz", 5, 3)
	startup := httpProbe(h.ReadinessPath, "/healthz", 2, 30)
	if h.ReadinessPath == "" {
		readiness = tcpProbe(5, 3)
		startup = tcpProbe(2, 30)
	}
	liveness := map[string]any{
		"enabled": false, "type": "http", "path": "/healthz",
		"initialDelaySeconds": 0, "periodSeconds": 10, "timeoutSeconds": 1, "failureThreshold": 3,
	}
	return map[string]any{"readiness": readiness, "startup": startup, "liveness": liveness}
}

func httpProbe(path, fallback string, period, failures int) map[string]any {
	p := path
	if p == "" {
		p = fallback
	}
	return map[string]any{
		"enabled": true, "type": "http", "path": p,
		"initialDelaySeconds": 0, "periodSeconds": period, "timeoutSeconds": 1, "failureThreshold": failures,
	}
}

func tcpProbe(period, failures int) map[string]any {
	return map[string]any{
		"enabled": true, "type": "tcp", "path": "/",
		"initialDelaySeconds": 0, "periodSeconds": period, "timeoutSeconds": 1, "failureThreshold": failures,
	}
}

// ingressValues maps exposure to the chart's ingress block. Internal exposure
// creates no route (ClusterIP only); LAN exposure selects the private entrypoint.
// Public exposure is not reachable here because the applications package rejects
// it in the core prototype.
func ingressValues(e platform.Exposure) map[string]any {
	ingress := map[string]any{
		"enabled":    false,
		"className":  "traefik",
		"entrypoint": "private",
		"hostname":   "",
		"tls":        map[string]any{"enabled": false, "secretName": ""},
	}
	if e.Mode == platform.ExposureLAN {
		ingress["enabled"] = true
		ingress["hostname"] = e.Hostname
	}
	return ingress
}

func orDefault(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}
