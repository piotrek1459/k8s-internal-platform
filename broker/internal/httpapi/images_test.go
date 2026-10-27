package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"k8s-internal-platform/broker/internal/platform"
)

type fakeInspector struct {
	report platform.ImageReport
	err    error
}

func (f fakeInspector) Inspect(context.Context, string, string) (platform.ImageReport, error) {
	return f.report, f.err
}

func routerWithInspector(insp platform.ImageInspector) http.Handler {
	return NewRouter(Config{Logger: discardLogger(), Inspector: insp}).Handler()
}

func TestInspectImageReturnsReport(t *testing.T) {
	insp := fakeInspector{report: platform.ImageReport{Digest: "sha256:abc", Architectures: []string{"arm64", "amd64"}, SuggestedPort: 8080}}
	h := routerWithInspector(insp)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/images/inspect", strings.NewReader(`{"image":"ghcr.io/example/app:1"}`))
	req.Header.Set("X-Requested-With", "fetch")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "sha256:abc") || !strings.Contains(rec.Body.String(), "8080") {
		t.Fatalf("unexpected body: %s", rec.Body.String())
	}
}

func TestInspectImageMapsArchErrorTo422(t *testing.T) {
	insp := fakeInspector{err: platform.FieldErrorf(platform.CodeImageArchUnsupported, "image", "no arm64")}
	h := routerWithInspector(insp)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/images/inspect", strings.NewReader(`{"image":"x"}`))
	req.Header.Set("X-Requested-With", "fetch")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", rec.Code)
	}
}

func TestInspectImageRequiresImage(t *testing.T) {
	h := routerWithInspector(fakeInspector{})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/images/inspect", strings.NewReader(`{}`))
	req.Header.Set("X-Requested-With", "fetch")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}
