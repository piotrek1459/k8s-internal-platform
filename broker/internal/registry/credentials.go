package registry

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strings"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"

	"k8s-internal-platform/broker/internal/platform"
)

// CredentialService keeps credential material in Kubernetes and metadata in SQLite.
type CredentialService struct {
	Store   platform.Store
	Secrets platform.RegistrySecretManager
}

func RegistryHost(host string) (string, error) {
	if strings.ContainsAny(host, "/@ \t\r\n") || host == "" {
		return "", platform.FieldErrorf(platform.CodeInvalidInput, "registry", "registry must be a hostname with an optional port")
	}
	reg, err := name.NewRegistry(host, name.StrictValidation)
	if err != nil {
		return "", platform.FieldErrorf(platform.CodeInvalidInput, "registry", "invalid registry hostname")
	}
	return reg.RegistryStr(), nil
}

func (s *CredentialService) Create(ctx context.Context, c platform.RegistryCredential, password string) error {
	host, err := RegistryHost(c.Registry)
	if err != nil {
		return err
	}
	c.Registry = host
	c.K8sSecretName = "registry-" + strings.ReplaceAll(c.ID, "_", "-")
	body, err := json.Marshal(map[string]any{"auths": map[string]any{host: map[string]string{"username": c.Username, "password": password, "auth": base64.StdEncoding.EncodeToString([]byte(c.Username + ":" + password))}}})
	if err != nil {
		return err
	}
	if err := s.Secrets.CreateRegistrySecret(ctx, c.K8sSecretName, body); err != nil {
		return err
	}
	if err := s.Store.CreateRegistryCredential(ctx, c); err != nil {
		_ = s.Secrets.DeleteRegistrySecret(ctx, c.K8sSecretName)
		return err
	}
	return nil
}

func (s *CredentialService) credential(ctx context.Context, id string) (platform.RegistryCredential, error) {
	creds, err := s.Store.ListRegistryCredentials(ctx)
	if err != nil {
		return platform.RegistryCredential{}, err
	}
	for _, c := range creds {
		if c.ID == id {
			return c, nil
		}
	}
	return platform.RegistryCredential{}, platform.Errorf(platform.CodeNotFound, "registry credential not found")
}

func (s *CredentialService) Resolve(ctx context.Context, id string) (authn.Authenticator, error) {
	c, err := s.credential(ctx, id)
	if err != nil {
		return nil, err
	}
	return s.ResolveForRegistry(ctx, id, c.Registry)
}

// ResolveForRegistry prevents sending a credential to an unrelated image host.
func (s *CredentialService) ResolveForRegistry(ctx context.Context, id, host string) (authn.Authenticator, error) {
	c, err := s.credential(ctx, id)
	if err != nil {
		return nil, err
	}
	host, err = RegistryHost(host)
	if err != nil {
		return nil, err
	}
	if c.Registry != host {
		return nil, platform.FieldErrorf(platform.CodeInvalidInput, "registryCredentialId", "credential belongs to another registry")
	}
	if c.K8sSecretName == "" {
		return nil, platform.Errorf(platform.CodeDependencyUnavailable, "credential has no stored authentication material; recreate it")
	}
	body, err := s.Secrets.ReadRegistrySecret(ctx, c.K8sSecretName)
	if err != nil {
		return nil, err
	}
	var config struct {
		Auths map[string]struct {
			Username string `json:"username"`
			Password string `json:"password"`
		} `json:"auths"`
	}
	if json.Unmarshal(body, &config) != nil {
		return nil, platform.Errorf(platform.CodeDependencyUnavailable, "stored registry credential is invalid")
	}
	auth, ok := config.Auths[host]
	if !ok || auth.Username == "" || auth.Password == "" {
		return nil, platform.Errorf(platform.CodeDependencyUnavailable, "stored registry credential is incomplete")
	}
	return authn.FromConfig(authn.AuthConfig{Username: auth.Username, Password: auth.Password}), nil
}

func (s *CredentialService) Delete(ctx context.Context, id string) error {
	c, err := s.credential(ctx, id)
	if err != nil {
		return err
	}
	// The store transaction refuses references from active and historical revisions.
	if err := s.Store.DeleteRegistryCredential(ctx, id); err != nil {
		return err
	}
	if c.K8sSecretName != "" {
		return s.Secrets.DeleteRegistrySecret(ctx, c.K8sSecretName)
	}
	return nil
}
