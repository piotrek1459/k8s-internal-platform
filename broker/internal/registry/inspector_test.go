package registry

import (
	"context"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/google/go-containerregistry/pkg/name"
	regpkg "github.com/google/go-containerregistry/pkg/registry"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/random"
	"github.com/google/go-containerregistry/pkg/v1/remote"

	"k8s-internal-platform/broker/internal/platform"
)

// startRegistry runs an in-memory OCI registry and returns its host:port. No
// network access leaves the test process.
func startRegistry(t *testing.T) string {
	t.Helper()
	srv := httptest.NewServer(regpkg.New())
	t.Cleanup(srv.Close)
	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatalf("parse registry url: %v", err)
	}
	return u.Host
}

// pushSingleArch pushes a one-architecture image and returns its reference.
func pushSingleArch(t *testing.T, host, repo, arch string) string {
	t.Helper()
	img, err := random.Image(1024, 1)
	if err != nil {
		t.Fatalf("random image: %v", err)
	}
	cfg, err := img.ConfigFile()
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	cfg = cfg.DeepCopy()
	cfg.Architecture = arch
	cfg.OS = "linux"
	img, err = mutate.ConfigFile(img, cfg)
	if err != nil {
		t.Fatalf("mutate config: %v", err)
	}
	ref := host + "/" + repo + ":tag"
	tag, _ := name.NewTag(ref)
	if err := remote.Write(tag, img); err != nil {
		t.Fatalf("push image: %v", err)
	}
	return ref
}

// pushMultiArch pushes an index containing the given architectures.
func pushMultiArch(t *testing.T, host, repo string, arches ...string) string {
	t.Helper()
	idx := v1.ImageIndex(empty.Index)
	for _, arch := range arches {
		img, err := random.Image(1024, 1)
		if err != nil {
			t.Fatalf("random image: %v", err)
		}
		idx = mutate.AppendManifests(idx, mutate.IndexAddendum{
			Add: img,
			Descriptor: v1.Descriptor{
				Platform: &v1.Platform{OS: "linux", Architecture: arch},
			},
		})
	}
	ref := host + "/" + repo + ":tag"
	tag, _ := name.NewTag(ref)
	if err := remote.WriteIndex(tag, idx); err != nil {
		t.Fatalf("push index: %v", err)
	}
	return ref
}

func TestInspectMultiArchResolvesDigestAndArches(t *testing.T) {
	host := startRegistry(t)
	ref := pushMultiArch(t, host, "demo/multi", "amd64", "arm64")

	insp := NewInspector(nil).WithHostArch("arm64")
	report, err := insp.Inspect(context.Background(), ref, "")
	if err != nil {
		t.Fatalf("inspect: %v", err)
	}
	if !strings.HasPrefix(report.Digest, "sha256:") {
		t.Fatalf("digest = %q", report.Digest)
	}
	if !contains(report.Architectures, "arm64") || !contains(report.Architectures, "amd64") {
		t.Fatalf("architectures = %v", report.Architectures)
	}
}

func TestInspectRejectsMissingHostArch(t *testing.T) {
	host := startRegistry(t)
	ref := pushMultiArch(t, host, "demo/amd-only", "amd64")

	insp := NewInspector(nil).WithHostArch("arm64")
	_, err := insp.Inspect(context.Background(), ref, "")
	derr, ok := err.(*platform.Error)
	if !ok || derr.Code != platform.CodeImageArchUnsupported {
		t.Fatalf("expected IMAGE_ARCH_UNSUPPORTED, got %v", err)
	}
	if derr.Field != "image" {
		t.Fatalf("field = %q, want image", derr.Field)
	}
}

func TestInspectSingleArchImage(t *testing.T) {
	host := startRegistry(t)
	ref := pushSingleArch(t, host, "demo/single", "arm64")

	insp := NewInspector(nil).WithHostArch("arm64")
	report, err := insp.Inspect(context.Background(), ref, "")
	if err != nil {
		t.Fatalf("inspect: %v", err)
	}
	if len(report.Architectures) != 1 || report.Architectures[0] != "arm64" {
		t.Fatalf("architectures = %v", report.Architectures)
	}
}

func TestInspectInvalidReferenceIsInvalidInput(t *testing.T) {
	insp := NewInspector(nil)
	_, err := insp.Inspect(context.Background(), "not a valid ref!!", "")
	derr, ok := err.(*platform.Error)
	if !ok || derr.Code != platform.CodeInvalidInput {
		t.Fatalf("expected INVALID_INPUT, got %v", err)
	}
}

func TestInspectUnreachableRegistryIsRetryable(t *testing.T) {
	// Point at a host with nothing listening; the transport error must map to the
	// retryable registry-unavailable code, never to a false compatibility claim.
	insp := NewInspector(nil).WithHostArch("arm64")
	_, err := insp.Inspect(context.Background(), "127.0.0.1:1/demo/x:tag", "")
	derr, ok := err.(*platform.Error)
	if !ok || derr.Code != platform.CodeRegistryUnavailable {
		t.Fatalf("expected REGISTRY_UNAVAILABLE, got %v", err)
	}
}

func contains(xs []string, want string) bool {
	for _, x := range xs {
		if x == want {
			return true
		}
	}
	return false
}
