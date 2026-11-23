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

func TestSecretDeletionReservationRejectsNewDependencies(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	seedSecret(t, s, "sec_reserve")
	if err := s.BeginSecretDeletion(ctx, "sec_reserve"); err != nil {
		t.Fatal(err)
	}
	meta, versions, err := s.GetSecret(ctx, "sec_reserve")
	if err != nil || !meta.Deleting || len(versions) != 1 {
		t.Fatalf("reservation lost metadata: %+v %v", meta, err)
	}
	assertCode(t, s.AddSecretVersion(ctx, platform.SecretVersion{SecretID: meta.ID, Version: 2}), platform.CodeConflict)
	_, err = s.AcceptOperation(ctx, platform.AcceptRequest{
		Type: platform.OperationCreate, IsNewApp: true,
		Application: platform.Application{ID: "app_reserved", Name: "app-reserved"},
		Revision: platform.Revision{ID: "rev_reserved", AppID: "app_reserved", Number: 1, Result: platform.RevisionPending,
			Spec: platform.RevisionSpec{SecretRefs: []platform.SecretRef{{SecretID: meta.ID, Version: 1}}}},
		Op: platform.Operation{ID: "op_reserved", AppID: "app_reserved", TargetRevision: "rev_reserved", Type: platform.OperationCreate},
	})
	assertCode(t, err, platform.CodeInvalidInput)
	var count int
	if err := s.db.sql.QueryRow(`SELECT COUNT(*) FROM applications WHERE id = 'app_reserved'`).Scan(&count); err != nil || count != 0 {
		t.Fatal("rejected dependency partially accepted an application")
	}
	if err := s.BeginSecretDeletion(ctx, meta.ID); err != nil {
		t.Fatalf("reservation is not retryable: %v", err)
	}
	if err := s.DeleteSecret(ctx, meta.ID); err != nil {
		t.Fatal(err)
	}
}

func TestSecretDeletionIgnoresDeletedApplicationHistory(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	seedSecret(t, s, "sec_history")
	_, err := s.AcceptOperation(ctx, platform.AcceptRequest{
		Type: platform.OperationCreate, IsNewApp: true,
		Application: platform.Application{ID: "app_history", Name: "app-history"},
		Revision: platform.Revision{ID: "rev_history", AppID: "app_history", Number: 1, Result: platform.RevisionSucceeded,
			Spec:   platform.RevisionSpec{SecretRefs: []platform.SecretRef{{SecretID: "sec_history", Version: 1}}},
			Frozen: &platform.FrozenSpec{SecretVersions: []platform.SecretRef{{SecretID: "sec_history", Version: 1}}}},
		Op: platform.Operation{ID: "op_history", AppID: "app_history", TargetRevision: "rev_history", Type: platform.OperationCreate},
	})
	if err != nil {
		t.Fatal(err)
	}
	assertCode(t, s.BeginSecretDeletion(ctx, "sec_history"), platform.CodeSecretInUse)
	// Represent a completed workload deletion; retained revisions are evidence,
	// not a rollback target once the application has been tombstoned.
	if _, err := s.db.sql.Exec(`UPDATE applications SET deleted = 1, active_operation_id = NULL WHERE id = 'app_history'`); err != nil {
		t.Fatal(err)
	}
	if err := s.BeginSecretDeletion(ctx, "sec_history"); err != nil {
		t.Fatalf("deleted history blocked removal: %v", err)
	}
	if err := s.DeleteSecret(ctx, "sec_history"); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := s.db.sql.QueryRow(`SELECT COUNT(*) FROM revisions WHERE id = 'rev_history'`).Scan(&count); err != nil || count != 1 {
		t.Fatal("Secret deletion removed retained revision evidence")
	}
}

func TestSecretReferenceMatchingDoesNotTreatIDsAsSQLPatterns(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	seedSecret(t, s, "sec_01")
	seedSecret(t, s, "secx01")
	_, err := s.AcceptOperation(ctx, platform.AcceptRequest{
		Type: platform.OperationCreate, IsNewApp: true,
		Application: platform.Application{ID: "app_exact", Name: "app-exact"},
		Revision: platform.Revision{ID: "rev_exact", AppID: "app_exact", Number: 1, Result: platform.RevisionPending,
			Spec: platform.RevisionSpec{SecretRefs: []platform.SecretRef{{SecretID: "secx01", Version: 1}}}},
		Op: platform.Operation{ID: "op_exact", AppID: "app_exact", TargetRevision: "rev_exact", Type: platform.OperationCreate},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteSecret(ctx, "sec_01"); err != nil {
		t.Fatalf("underscore matched another Secret ID: %v", err)
	}
	assertCode(t, s.BeginSecretDeletion(ctx, "secx01"), platform.CodeSecretInUse)
}

func TestSecretDeletionHandlesLegacyEmptyFrozenReferences(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	seedSecret(t, s, "sec_unused")
	_, err := s.AcceptOperation(ctx, platform.AcceptRequest{
		Type: platform.OperationCreate, IsNewApp: true,
		Application: platform.Application{ID: "app_empty", Name: "app-empty"},
		Revision:    platform.Revision{ID: "rev_empty", AppID: "app_empty", Number: 1, Result: platform.RevisionPending},
		Op:          platform.Operation{ID: "op_empty", AppID: "app_empty", TargetRevision: "rev_empty", Type: platform.OperationCreate},
	})
	if err != nil {
		t.Fatal(err)
	}
	// Existing execution writes an empty SQL string for no frozen Secret refs.
	if err := s.FreezeExecutionSpec(ctx, "rev_empty", platform.FrozenSpec{ImageDigest: "image@sha256:fixture"}); err != nil {
		t.Fatal(err)
	}
	if err := s.BeginSecretDeletion(ctx, "sec_unused"); err != nil {
		t.Fatalf("unrelated empty frozen reference broke deletion: %v", err)
	}
}
