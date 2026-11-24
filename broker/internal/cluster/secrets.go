package cluster

import (
	"context"
	"crypto/subtle"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"k8s-internal-platform/broker/internal/platform"
)

var _ platform.SecretManager = (*Client)(nil)

// secretName builds the deterministic, versioned Kubernetes Secret name for a
// platform secret. Including the version makes each version a distinct immutable
// object, so a referenced version is never mutated in place.
func secretName(secretID string, version int) string {
	return fmt.Sprintf("app-secret-%s-v%d", sanitizeID(secretID), version)
}

// CreateImmutableSecret creates an immutable Secret carrying the configuration
// data for one secret version. It is idempotent for retries: if the exact
// versioned Secret already exists, its existing name and UID are returned rather
// than failing, so a re-executed operation converges.
func (c *Client) CreateImmutableSecret(ctx context.Context, secretID string, version int, data map[string][]byte) (string, string, error) {
	name := secretName(secretID, version)
	immutable := true
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: c.namespace,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by":  "k8s-internal-platform",
				"platform.local/secret-id":      sanitizeID(secretID),
				"platform.local/secret-version": fmt.Sprintf("%d", version),
			},
		},
		Immutable: &immutable,
		Data:      data,
		Type:      corev1.SecretTypeOpaque,
	}
	created, err := c.cs.CoreV1().Secrets(c.namespace).Create(ctx, secret, metav1.CreateOptions{})
	if apierrors.IsAlreadyExists(err) {
		// A same-name object already exists. This is a safe idempotent retry only
		// when its contents match the request; otherwise a different request is
		// trying to bind this version name to different data, which must not be
		// acknowledged as success.
		existing, getErr := c.cs.CoreV1().Secrets(c.namespace).Get(ctx, name, metav1.GetOptions{})
		if getErr != nil {
			return "", "", platform.Errorf(platform.CodeDependencyUnavailable, "could not read existing configuration Secret")
		}
		if !secretDataEqual(existing.Data, data) {
			return "", "", platform.Errorf(platform.CodeConflict,
				"secret version %q already exists with different contents", name)
		}
		return existing.Name, string(existing.UID), nil
	}
	if err != nil {
		return "", "", platform.Errorf(platform.CodeDependencyUnavailable, "could not create configuration Secret")
	}
	return created.Name, string(created.UID), nil
}

// DeleteImmutableSecret refuses name reuse and ownership changes. Preconditions
// also protect the interval between reading the object and issuing the delete.
func (c *Client) DeleteImmutableSecret(ctx context.Context, version platform.SecretVersion) error {
	if version.SecretID == "" || version.Version < 1 || version.K8sSecretUID == "" ||
		version.K8sSecretName != secretName(version.SecretID, version.Version) {
		return platform.Errorf(platform.CodeConflict, "recorded Secret identity is incomplete or inconsistent")
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	secret, err := c.cs.CoreV1().Secrets(c.namespace).Get(ctx, version.K8sSecretName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return platform.Errorf(platform.CodeDependencyUnavailable, "read configuration Secret for deletion failed; retry deletion")
	}
	if string(secret.UID) != version.K8sSecretUID || secret.ResourceVersion == "" || secret.Namespace != c.namespace ||
		secret.Type != corev1.SecretTypeOpaque || secret.Immutable == nil || !*secret.Immutable ||
		secret.Labels["app.kubernetes.io/managed-by"] != "k8s-internal-platform" ||
		secret.Labels["platform.local/secret-id"] != sanitizeID(version.SecretID) ||
		secret.Labels["platform.local/secret-version"] != fmt.Sprint(version.Version) {
		return platform.Errorf(platform.CodeConflict, "configuration Secret identity or ownership changed; deletion remains pending")
	}
	if len(secret.Finalizers) != 0 {
		return platform.Errorf(platform.CodeConflict, "configuration Secret has finalizers; resolve them before retrying deletion")
	}
	uid := types.UID(version.K8sSecretUID)
	err = c.cs.CoreV1().Secrets(c.namespace).Delete(ctx, secret.Name, metav1.DeleteOptions{
		Preconditions: &metav1.Preconditions{UID: &uid, ResourceVersion: &secret.ResourceVersion},
	})
	if apierrors.IsNotFound(err) {
		return nil
	}
	if apierrors.IsConflict(err) {
		return platform.Errorf(platform.CodeConflict, "configuration Secret changed during deletion; retry deletion")
	}
	if err != nil {
		return platform.Errorf(platform.CodeDependencyUnavailable, "configuration Secret removal failed; retry deletion")
	}
	remaining, err := c.cs.CoreV1().Secrets(c.namespace).Get(ctx, secret.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err == nil && string(remaining.UID) != version.K8sSecretUID {
		return platform.Errorf(platform.CodeConflict, "configuration Secret name was reused; deletion remains pending")
	}
	return platform.Errorf(platform.CodeDependencyUnavailable, "configuration Secret absence could not be verified; retry deletion")
}

// secretDataEqual reports whether two secret data maps hold the same keys and
// byte values. It compares in constant-ish fashion over the full content; the
// comparison does not short-circuit on the first differing byte for a given key.
func secretDataEqual(a, b map[string][]byte) bool {
	if len(a) != len(b) {
		return false
	}
	for k, av := range a {
		bv, ok := b[k]
		if !ok || subtle.ConstantTimeCompare(av, bv) != 1 {
			return false
		}
	}
	return true
}

// sanitizeID lowercases and replaces characters invalid in a DNS-1123 subdomain
// so an opaque platform ID (which may contain underscores) yields a valid
// Kubernetes object name component.
func sanitizeID(id string) string {
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
