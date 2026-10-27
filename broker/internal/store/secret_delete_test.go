package store

import (
	"context"
	"testing"

	"k8s-internal-platform/broker/internal/platform"
)

// seedSecret creates a secret with one version directly in the store.
func seedSecret(t *testing.T, s *Store, id string) {
	t.Helper()
	err := s.CreateSecret(context.Background(),
		platform.SecretMetadata{ID: id, Owner: "admin", Name: "name-" + id},
		platform.SecretVersion{SecretID: id, Version: 1, Keys: []string{"value"}, K8sSecretName: "k8s-" + id, K8sSecretUID: "uid"})
	if err != nil {
		t.Fatalf("seed secret: %v", err)
	}
}

func TestDeleteSecretBlockedByQueuedUnfrozenReference(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	seedSecret(t, s, "sec_01")

	// Accept an application whose (unfrozen) revision references the secret, and
	// leave the operation queued. Deletion must be refused even though no frozen
	// revision references it yet.
	ref := []platform.SecretRef{{EnvironmentVariable: "APP_SECRET", SecretID: "sec_01", Version: 1}}
	_, err := s.AcceptOperation(ctx, platform.AcceptRequest{
		Type:        platform.OperationCreate,
		Application: platform.Application{ID: "app1", Name: "app1"},
		IsNewApp:    true,
		Revision: platform.Revision{ID: "rev1", AppID: "app1", Number: 1, Result: platform.RevisionPending, Actor: "t",
			Spec: platform.RevisionSpec{Image: "img:1", Profile: platform.ProfileHTTP, ContainerPort: 8080, SecretRefs: ref}},
		Op:          platform.Operation{ID: "op1", AppID: "app1", Type: platform.OperationCreate, TargetRevision: "rev1"},
		RequestHash: "h",
	})
	if err != nil {
		t.Fatalf("accept: %v", err)
	}

	err = s.DeleteSecret(ctx, "sec_01")
	assertCode(t, err, platform.CodeSecretInUse)

	// The secret must still exist and its metadata be intact.
	if _, _, err := s.GetSecret(ctx, "sec_01"); err != nil {
		t.Fatalf("secret was removed despite an in-flight reference: %v", err)
	}
}

func TestDeleteSecretBlockedByNeedsAttentionReference(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	seedSecret(t, s, "sec_02")

	ref := []platform.SecretRef{{EnvironmentVariable: "APP_SECRET", SecretID: "sec_02", Version: 1}}
	_, err := s.AcceptOperation(ctx, platform.AcceptRequest{
		Type:        platform.OperationCreate,
		Application: platform.Application{ID: "app2", Name: "app2"},
		IsNewApp:    true,
		Revision: platform.Revision{ID: "rev2", AppID: "app2", Number: 1, Result: platform.RevisionPending, Actor: "t",
			Spec: platform.RevisionSpec{Image: "img:1", Profile: platform.ProfileHTTP, ContainerPort: 8080, SecretRefs: ref}},
		Op:          platform.Operation{ID: "op2", AppID: "app2", Type: platform.OperationCreate, TargetRevision: "rev2"},
		RequestHash: "h",
	})
	if err != nil {
		t.Fatalf("accept: %v", err)
	}
	// Move the operation to NeedsAttention; its recovery dependency must stay
	// protected regardless of the candidate revision's result.
	if err := s.TransitionOperation(ctx, "op2", platform.OperationNeedsAttention, "stuck", "unresolved"); err != nil {
		t.Fatalf("transition: %v", err)
	}
	if err := s.MarkRevisionResult(ctx, "rev2", platform.RevisionFailed); err != nil {
		t.Fatalf("mark: %v", err)
	}

	err = s.DeleteSecret(ctx, "sec_02")
	assertCode(t, err, platform.CodeSecretInUse)
}

func TestDeleteSecretSucceedsWhenUnreferenced(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	seedSecret(t, s, "sec_free")
	if err := s.DeleteSecret(ctx, "sec_free"); err != nil {
		t.Fatalf("unreferenced secret should delete: %v", err)
	}
	if _, _, err := s.GetSecret(ctx, "sec_free"); err == nil {
		t.Fatal("secret still present after deletion")
	}
}
