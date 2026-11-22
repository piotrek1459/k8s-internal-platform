package registry

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	registryserver "github.com/google/go-containerregistry/pkg/registry"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"k8s-internal-platform/broker/internal/platform"
	"k8s-internal-platform/broker/internal/store"
)

type memorySecrets struct{ values map[string][]byte }

func (m memorySecrets) CreateRegistrySecret(_ context.Context, n string, b []byte) error {
	m.values[n] = b
	return nil
}
func (m memorySecrets) ReadRegistrySecret(_ context.Context, n string) ([]byte, error) {
	return m.values[n], nil
}
func (m memorySecrets) DeleteRegistrySecret(_ context.Context, n string) error {
	delete(m.values, n)
	return nil
}

func TestCredentialMaterialIsStoredAndScopedToRegistry(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(ctx, t.TempDir()+"/platform.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	st := store.New(db)
	secret := memorySecrets{map[string][]byte{}}
	svc := &CredentialService{Store: st, Secrets: secret}
	c := platform.RegistryCredential{ID: "cred_test", Registry: "registry.example.com", Username: "operator", Label: "test"}
	if err := svc.Create(ctx, c, "private-password"); err != nil {
		t.Fatal(err)
	}
	auth, err := svc.ResolveForRegistry(ctx, c.ID, c.Registry)
	if err != nil {
		t.Fatal(err)
	}
	config, _ := auth.Authorization()
	if config.Password != "private-password" {
		t.Fatal("password was discarded")
	}
	if _, err := svc.ResolveForRegistry(ctx, c.ID, "unrelated.example.com"); err == nil {
		t.Fatal("credential exposed to another registry")
	}
	metadata, _ := st.ListRegistryCredentials(ctx)
	body, _ := json.Marshal(metadata)
	if strings.Contains(string(body), "private-password") {
		t.Fatal("secret leaked into metadata")
	}
	if err := svc.Delete(ctx, c.ID); err != nil {
		t.Fatal(err)
	}
	if len(secret.values) != 0 {
		t.Fatal("credential secret remains after deletion")
	}
}

func TestPrivateInspectorUsesStoredPassword(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(ctx, t.TempDir()+"/platform.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	st := store.New(db)
	svc := &CredentialService{Store: st, Secrets: memorySecrets{map[string][]byte{}}}
	handler := registryserver.New()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if !ok || user != "operator" || pass != "private-password" {
			w.Header().Set("WWW-Authenticate", `Basic realm="private"`)
			w.WriteHeader(401)
			return
		}
		handler.ServeHTTP(w, r)
	}))
	defer server.Close()
	host := strings.TrimPrefix(server.URL, "http://")
	tag, _ := name.NewTag(host+"/private/app:v1", name.Insecure)
	if err := remote.Write(tag, empty.Image, remote.WithAuth(authn.FromConfig(authn.AuthConfig{Username: "operator", Password: "private-password"}))); err != nil {
		t.Fatal(err)
	}
	if err := svc.Create(ctx, platform.RegistryCredential{ID: "cred_private", Registry: host, Username: "operator"}, "private-password"); err != nil {
		t.Fatal(err)
	}
	inspector := NewInspector(svc).WithInsecureRegistries([]string{host}).WithHostArch("amd64")
	// empty.Image has no architecture; authentication success reaches the
	// architecture check instead of being mistaken for a registry outage.
	_, err = inspector.Inspect(ctx, tag.Name(), "cred_private")
	var domain *platform.Error
	if !errors.As(err, &domain) || domain.Code != platform.CodeImageArchUnsupported {
		t.Fatalf("private authentication failed: %v", err)
	}
	if _, err := inspector.Inspect(ctx, tag.Name(), ""); err == nil {
		t.Fatal("anonymous private inspection succeeded")
	}
}
