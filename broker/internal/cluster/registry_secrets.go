package cluster

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s-internal-platform/broker/internal/platform"
)

func (c *Client) CreateRegistrySecret(ctx context.Context, name string, data []byte) error {
	immutable := true
	_, err := c.cs.CoreV1().Secrets(c.namespace).Create(ctx, &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: c.namespace, Labels: map[string]string{"platform.local/credential": "true"}},
		Type:       corev1.SecretTypeDockerConfigJson, Immutable: &immutable,
		Data: map[string][]byte{corev1.DockerConfigJsonKey: data},
	}, metav1.CreateOptions{})
	if err != nil {
		return platform.Errorf(platform.CodeDependencyUnavailable, "could not store registry authentication")
	}
	return nil
}
func (c *Client) ReadRegistrySecret(ctx context.Context, name string) ([]byte, error) {
	secret, err := c.cs.CoreV1().Secrets(c.namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, platform.Errorf(platform.CodeDependencyUnavailable, "could not read registry authentication")
	}
	return secret.Data[corev1.DockerConfigJsonKey], nil
}
func (c *Client) DeleteRegistrySecret(ctx context.Context, name string) error {
	err := c.cs.CoreV1().Secrets(c.namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return platform.Errorf(platform.CodeDependencyUnavailable, "could not remove registry authentication")
	}
	return nil
}
