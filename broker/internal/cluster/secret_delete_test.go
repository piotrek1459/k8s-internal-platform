package cluster

import (
	"context"
	"errors"
	"testing"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/fake"
	ktesting "k8s.io/client-go/testing"

	"k8s-internal-platform/broker/internal/platform"
)

func deletionSecret() (*corev1.Secret, platform.SecretVersion) {
	secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{
		Name: "app-secret-sec-delete-v2", Namespace: ns, UID: "recorded-uid", ResourceVersion: "41",
		Labels: map[string]string{"app.kubernetes.io/managed-by": "k8s-internal-platform",
			"platform.local/secret-id": "sec-delete", "platform.local/secret-version": "2"},
	}, Type: corev1.SecretTypeOpaque, Immutable: ptrBool(true)}
	return secret, platform.SecretVersion{SecretID: "sec_delete", Version: 2,
		K8sSecretName: secret.Name, K8sSecretUID: string(secret.UID)}
}

func TestDeleteImmutableSecretGuardsUIDAndResourceVersion(t *testing.T) {
	secret, version := deletionSecret()
	cs := fake.NewSimpleClientset(secret)
	cs.PrependReactor("delete", "secrets", func(action ktesting.Action) (bool, runtime.Object, error) {
		options := action.(ktesting.DeleteAction).GetDeleteOptions()
		if action.GetNamespace() != ns || options.Preconditions == nil || options.Preconditions.UID == nil ||
			string(*options.Preconditions.UID) != version.K8sSecretUID || options.Preconditions.ResourceVersion == nil ||
			*options.Preconditions.ResourceVersion != "41" {
			t.Fatal("delete lacked recorded identity/concurrency preconditions")
		}
		return false, nil, nil
	})
	c := NewWithClientset(cs, ns)
	if err := c.DeleteImmutableSecret(context.Background(), version); err != nil {
		t.Fatal(err)
	}
	// The first removal may have committed before the client disconnected.
	if err := c.DeleteImmutableSecret(context.Background(), version); err != nil {
		t.Fatalf("missing-version retry: %v", err)
	}
}

func TestDeleteImmutableSecretRefusesChangedOwnershipAndNameReuse(t *testing.T) {
	for _, scenario := range []string{"uid", "resource-version", "managed-by", "secret-id", "version-label", "type", "mutable", "finalizer", "recorded-name", "missing-recorded-uid"} {
		t.Run(scenario, func(t *testing.T) {
			secret, version := deletionSecret()
			switch scenario {
			case "uid":
				secret.UID = "replacement-uid"
			case "resource-version":
				secret.ResourceVersion = ""
			case "managed-by":
				secret.Labels["app.kubernetes.io/managed-by"] = "another-owner"
			case "secret-id":
				secret.Labels["platform.local/secret-id"] = "another-secret"
			case "version-label":
				secret.Labels["platform.local/secret-version"] = "3"
			case "type":
				secret.Type = corev1.SecretTypeDockerConfigJson
			case "mutable":
				secret.Immutable = nil
			case "finalizer":
				secret.Finalizers = []string{"example.com/retain"}
			case "recorded-name":
				version.K8sSecretName = "unrelated-secret"
			case "missing-recorded-uid":
				version.K8sSecretUID = ""
			}
			cs := fake.NewSimpleClientset(secret)
			err := NewWithClientset(cs, ns).DeleteImmutableSecret(context.Background(), version)
			var pe *platform.Error
			if !errors.As(err, &pe) || pe.Code != platform.CodeConflict {
				t.Fatalf("unsafe identity accepted: %v", err)
			}
			for _, action := range cs.Actions() {
				if action.GetVerb() != "get" {
					t.Fatal("ownership refusal issued a mutation")
				}
			}
		})
	}
}

func TestDeleteImmutableSecretDoesNotAcknowledgeUnverifiedRemoval(t *testing.T) {
	secret, version := deletionSecret()
	cs := fake.NewSimpleClientset(secret)
	cs.PrependReactor("delete", "secrets", func(ktesting.Action) (bool, runtime.Object, error) {
		// A successful transport response is not evidence that a finalizer or
		// interrupted API transaction actually removed the value.
		return true, nil, nil
	})
	err := NewWithClientset(cs, ns).DeleteImmutableSecret(context.Background(), version)
	var pe *platform.Error
	if !errors.As(err, &pe) || pe.Code != platform.CodeDependencyUnavailable {
		t.Fatalf("retained value acknowledged as deleted: %v", err)
	}
}

func TestDeleteImmutableSecretConcurrentChangeIsRetryable(t *testing.T) {
	secret, version := deletionSecret()
	cs := fake.NewSimpleClientset(secret)
	cs.PrependReactor("delete", "secrets", func(ktesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewConflict(schema.GroupResource{Resource: "secrets"}, secret.Name, errors.New("injected change"))
	})
	err := NewWithClientset(cs, ns).DeleteImmutableSecret(context.Background(), version)
	var pe *platform.Error
	if !errors.As(err, &pe) || pe.Code != platform.CodeConflict {
		t.Fatalf("concurrent change did not retain refusal: %v", err)
	}
}
