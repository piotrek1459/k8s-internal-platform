// Package registry inspects container image references before deployment. It
// resolves an image's digest and the architectures it provides, enforcing that
// the host architecture is available so an incompatible image is rejected before
// any cluster mutation. It implements the platform.ImageInspector port.
package registry

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"sort"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"

	"k8s-internal-platform/broker/internal/platform"
)

// CredentialResolver returns registry credentials for a stored credential ID. A
// nil resolver, or an empty credential ID, uses anonymous access (public images).
type CredentialResolver interface {
	Resolve(ctx context.Context, credentialID string) (authn.Authenticator, error)
}

// Inspector resolves image metadata from a remote registry. hostArch is the
// architecture the cluster nodes run (for example "arm64" on the reference Pi);
// an image lacking a linux/<hostArch> variant is rejected.
type Inspector struct {
	hostArch  string
	resolver  CredentialResolver
	transport remote.Option // optional override for tests (e.g. a fake registry)
}

// NewInspector builds an Inspector for the current host architecture. The
// resolver may be nil for anonymous-only access.
func NewInspector(resolver CredentialResolver) *Inspector {
	return &Inspector{hostArch: runtime.GOARCH, resolver: resolver}
}

// WithHostArch overrides the required host architecture (used by tests and by
// callers targeting a cluster whose architecture differs from the broker's).
func (i *Inspector) WithHostArch(arch string) *Inspector {
	i.hostArch = arch
	return i
}

// WithRemoteOption injects a remote option, letting tests point the inspector at
// an in-process fake registry.
func (i *Inspector) WithRemoteOption(opt remote.Option) *Inspector {
	i.transport = opt
	return i
}

var _ platform.ImageInspector = (*Inspector)(nil)

// Inspect resolves the image reference, verifies the host architecture is
// available, and returns the digest and the set of architectures. Registry or
// network failures are reported as retryable CodeRegistryUnavailable; a genuine
// architecture mismatch is CodeImageArchUnsupported.
func (i *Inspector) Inspect(ctx context.Context, imageRef, registryCredentialID string) (platform.ImageReport, error) {
	ref, err := name.ParseReference(imageRef)
	if err != nil {
		return platform.ImageReport{}, platform.FieldErrorf(platform.CodeInvalidInput, "image",
			"invalid image reference: %v", err)
	}

	opts := []remote.Option{remote.WithContext(ctx)}
	if i.transport != nil {
		opts = append(opts, i.transport)
	}
	auth, err := i.authFor(ctx, registryCredentialID)
	if err != nil {
		return platform.ImageReport{}, err
	}
	opts = append(opts, remote.WithAuth(auth))

	desc, err := remote.Get(ref, opts...)
	if err != nil {
		return platform.ImageReport{}, classifyRemoteError(err)
	}

	report := platform.ImageReport{Digest: desc.Digest.String()}

	// A multi-platform image is an index listing per-platform manifests; a
	// single-platform image is one manifest whose config declares its arch.
	if desc.MediaType.IsIndex() {
		idx, err := desc.ImageIndex()
		if err != nil {
			return platform.ImageReport{}, classifyRemoteError(err)
		}
		report.Architectures, err = indexArchitectures(idx)
		if err != nil {
			return platform.ImageReport{}, classifyRemoteError(err)
		}
	} else {
		img, err := desc.Image()
		if err != nil {
			return platform.ImageReport{}, classifyRemoteError(err)
		}
		cfg, err := img.ConfigFile()
		if err != nil {
			return platform.ImageReport{}, classifyRemoteError(err)
		}
		// Only a Linux image is supported. A single-manifest image for another OS
		// (for example windows/amd64) must be rejected here rather than passing
		// preflight and failing during workload execution.
		if cfg.OS == "linux" {
			report.Architectures = []string{cfg.Architecture}
		}
		report.SuggestedPort = firstExposedPort(cfg)
	}

	if !containsArch(report.Architectures, i.hostArch) {
		return platform.ImageReport{}, platform.FieldErrorf(platform.CodeImageArchUnsupported, "image",
			"image has no linux/%s variant (available linux platforms: %v)", i.hostArch, report.Architectures)
	}
	return report, nil
}

func (i *Inspector) authFor(ctx context.Context, credentialID string) (authn.Authenticator, error) {
	if credentialID == "" || i.resolver == nil {
		return authn.Anonymous, nil
	}
	auth, err := i.resolver.Resolve(ctx, credentialID)
	if err != nil {
		return nil, err
	}
	if auth == nil {
		return authn.Anonymous, nil
	}
	return auth, nil
}

func indexArchitectures(idx v1.ImageIndex) ([]string, error) {
	manifest, err := idx.IndexManifest()
	if err != nil {
		return nil, err
	}
	set := map[string]bool{}
	for _, m := range manifest.Manifests {
		if m.Platform == nil {
			continue
		}
		// Only Linux variants are relevant; attestation/unknown entries are skipped.
		if m.Platform.OS != "" && m.Platform.OS != "linux" {
			continue
		}
		if m.Platform.Architecture == "" || m.Platform.Architecture == "unknown" {
			continue
		}
		set[m.Platform.Architecture] = true
	}
	return sortedKeys(set), nil
}

func firstExposedPort(cfg *v1.ConfigFile) int {
	if cfg == nil || cfg.Config.ExposedPorts == nil {
		return 0
	}
	// ExposedPorts keys look like "8080/tcp"; return the lowest numeric port as a
	// hint only. The administrator must still confirm it.
	best := 0
	for spec := range cfg.Config.ExposedPorts {
		var port int
		if _, err := fmt.Sscanf(spec, "%d", &port); err == nil && port > 0 {
			if best == 0 || port < best {
				best = port
			}
		}
	}
	return best
}

func containsArch(arches []string, want string) bool {
	for _, a := range arches {
		if a == want {
			return true
		}
	}
	return false
}

func sortedKeys(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// classifyRemoteError maps registry transport errors to domain errors. Transport
// and HTTP errors are retryable (CodeRegistryUnavailable); the caller must not
// claim compatibility when inspection could not complete.
func classifyRemoteError(err error) error {
	var transportErr *transportError
	if errors.As(err, &transportErr) {
		return platform.Errorf(platform.CodeRegistryUnavailable, "registry request failed: %v", err)
	}
	return platform.Errorf(platform.CodeRegistryUnavailable, "could not inspect image: %v", err)
}

// transportError is a sentinel type the tests can wrap to assert the retryable
// classification path without a real network stack.
type transportError struct{ error }
