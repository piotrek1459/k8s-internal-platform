package applications

import (
	"k8s-internal-platform/broker/internal/platform"
)

// NewUpdateRequest builds the AcceptRequest for updating an existing application.
// The caller provides the full new spec (already validated by ValidateUpdate) and
// the application's current version for the optimistic-lock check.
func (f Factory) NewUpdateRequest(app platform.Application, spec platform.RevisionSpec, expectVersion int64, idempotencyKey, requestHash, actor, requestID string) platform.AcceptRequest {
	revID := f.IDs.NewID("rev")
	opID := f.IDs.NewID("op")
	now := f.Clock.Now()

	rev := platform.Revision{
		ID:      revID,
		AppID:   app.ID,
		Number:  app.Version + 1,
		Spec:    spec,
		Result:  platform.RevisionPending,
		Actor:   actor,
		Created: now,
	}
	op := platform.Operation{
		ID:             opID,
		AppID:          app.ID,
		Type:           platform.OperationUpdate,
		SourceRevision: app.DesiredRevisionID,
		TargetRevision: revID,
		State:          platform.OperationQueued,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	return platform.AcceptRequest{
		Type:           platform.OperationUpdate,
		Application:    app,
		IsNewApp:       false,
		ExpectVersion:  expectVersion,
		Revision:       rev,
		Op:             op,
		IdempotencyKey: idempotencyKey,
		RequestHash:    requestHash,
		Actor:          actor,
		RequestID:      requestID,
	}
}

// NewRestartRequest builds the AcceptRequest for restarting an application with
// its current spec unchanged. The caller must supply the current desired revision's
// spec so the worker can apply (and re-freeze) it.
func (f Factory) NewRestartRequest(app platform.Application, currentSpec platform.RevisionSpec, expectVersion int64, idempotencyKey, requestHash, actor, requestID string) platform.AcceptRequest {
	revID := f.IDs.NewID("rev")
	opID := f.IDs.NewID("op")
	now := f.Clock.Now()

	rev := platform.Revision{
		ID:      revID,
		AppID:   app.ID,
		Number:  app.Version + 1,
		Spec:    currentSpec,
		Result:  platform.RevisionPending,
		Actor:   actor,
		Created: now,
	}
	op := platform.Operation{
		ID:             opID,
		AppID:          app.ID,
		Type:           platform.OperationRestart,
		SourceRevision: app.DesiredRevisionID,
		TargetRevision: revID,
		State:          platform.OperationQueued,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	return platform.AcceptRequest{
		Type:           platform.OperationRestart,
		Application:    app,
		IsNewApp:       false,
		ExpectVersion:  expectVersion,
		Revision:       rev,
		Op:             op,
		IdempotencyKey: idempotencyKey,
		RequestHash:    requestHash,
		Actor:          actor,
		RequestID:      requestID,
	}
}

// NewRollbackRequest builds the AcceptRequest for rolling back to a prior
// succeeded revision. The caller validates that targetRevision belongs to this app
// and has result=succeeded before calling this. The frozen spec from the target
// revision is copied so no new image inspection is needed.
func (f Factory) NewRollbackRequest(app platform.Application, targetRevision platform.Revision, expectVersion int64, idempotencyKey, requestHash, actor, requestID string) platform.AcceptRequest {
	revID := f.IDs.NewID("rev")
	opID := f.IDs.NewID("op")
	now := f.Clock.Now()

	rev := platform.Revision{
		ID:      revID,
		AppID:   app.ID,
		Number:  app.Version + 1,
		Spec:    targetRevision.Spec,
		Frozen:  targetRevision.Frozen, // reuse frozen spec; no re-inspection
		Result:  platform.RevisionPending,
		Actor:   actor,
		Created: now,
	}
	op := platform.Operation{
		ID:             opID,
		AppID:          app.ID,
		Type:           platform.OperationRollback,
		SourceRevision: app.DesiredRevisionID,
		TargetRevision: revID,
		State:          platform.OperationQueued,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	return platform.AcceptRequest{
		Type:           platform.OperationRollback,
		Application:    app,
		IsNewApp:       false,
		ExpectVersion:  expectVersion,
		Revision:       rev,
		Op:             op,
		IdempotencyKey: idempotencyKey,
		RequestHash:    requestHash,
		Actor:          actor,
		RequestID:      requestID,
	}
}

// NewReapplyRequest builds the AcceptRequest for reapplying the current desired
// spec verbatim (drift recovery). The caller supplies the current revision's spec.
func (f Factory) NewReapplyRequest(app platform.Application, currentSpec platform.RevisionSpec, expectVersion int64, idempotencyKey, requestHash, actor, requestID string) platform.AcceptRequest {
	revID := f.IDs.NewID("rev")
	opID := f.IDs.NewID("op")
	now := f.Clock.Now()

	rev := platform.Revision{
		ID:      revID,
		AppID:   app.ID,
		Number:  app.Version + 1,
		Spec:    currentSpec,
		Result:  platform.RevisionPending,
		Actor:   actor,
		Created: now,
	}
	op := platform.Operation{
		ID:             opID,
		AppID:          app.ID,
		Type:           platform.OperationReapply,
		SourceRevision: app.DesiredRevisionID,
		TargetRevision: revID,
		State:          platform.OperationQueued,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	return platform.AcceptRequest{
		Type:           platform.OperationReapply,
		Application:    app,
		IsNewApp:       false,
		ExpectVersion:  expectVersion,
		Revision:       rev,
		Op:             op,
		IdempotencyKey: idempotencyKey,
		RequestHash:    requestHash,
		Actor:          actor,
		RequestID:      requestID,
	}
}

// NewDeleteRequest builds the AcceptRequest for deleting an application.
// A minimal revision is created so AcceptOperation can record the operation
// normally; the worker calls helm.Uninstall and does not use the spec.
func (f Factory) NewDeleteRequest(app platform.Application, expectVersion int64, idempotencyKey, requestHash, actor, requestID string) platform.AcceptRequest {
	revID := f.IDs.NewID("rev")
	opID := f.IDs.NewID("op")
	now := f.Clock.Now()

	rev := platform.Revision{
		ID:      revID,
		AppID:   app.ID,
		Number:  app.Version + 1,
		Spec:    platform.RevisionSpec{}, // not used by delete
		Result:  platform.RevisionPending,
		Actor:   actor,
		Created: now,
	}
	op := platform.Operation{
		ID:             opID,
		AppID:          app.ID,
		Type:           platform.OperationDelete,
		SourceRevision: app.DesiredRevisionID,
		TargetRevision: revID,
		State:          platform.OperationQueued,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	return platform.AcceptRequest{
		Type:           platform.OperationDelete,
		Application:    app,
		IsNewApp:       false,
		ExpectVersion:  expectVersion,
		Revision:       rev,
		Op:             op,
		IdempotencyKey: idempotencyKey,
		RequestHash:    requestHash,
		Actor:          actor,
		RequestID:      requestID,
	}
}
