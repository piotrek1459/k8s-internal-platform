// Package operations runs the broker's durable work: a single worker claims
// persisted operations one at a time, freezes each operation's execution digest
// before any cluster mutation, applies the change through Helm, and records the
// outcome. It depends only on the platform ports, so the whole execution and
// recovery state machine is unit-testable without a cluster. Startup recovery of
// interrupted operations is in recovery.go.
package operations

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"k8s-internal-platform/broker/internal/platform"
)

// pollInterval is the backstop cadence at which the worker looks for claimable
// work even without a wake signal, so operations recovered at startup drain and a
// missed wake never strands work.
const pollInterval = 5 * time.Second

// Worker executes persisted operations serially. One Worker runs per broker
// process; the store's claim guarantees at most one operation per application and
// skips applications blocked by an unresolved NeedsAttention operation.
type Worker struct {
	store     platform.Store
	helm      platform.HelmInstaller
	observer  platform.ClusterObserver
	inspector platform.ImageInspector
	clock     platform.Clock
	logger    *slog.Logger

	wake chan struct{}
}

// NewWorker builds a worker. observer and inspector may be nil in narrow tests
// that do not reach the steps using them.
func NewWorker(store platform.Store, helm platform.HelmInstaller, observer platform.ClusterObserver, inspector platform.ImageInspector, clock platform.Clock, logger *slog.Logger) *Worker {
	if logger == nil {
		logger = slog.Default()
	}
	return &Worker{
		store: store, helm: helm, observer: observer, inspector: inspector,
		clock: clock, logger: logger,
		wake: make(chan struct{}, 1),
	}
}

// Wake signals the worker that new work may be available. It never blocks: a
// pending wake already covers the next drain.
func (w *Worker) Wake() {
	select {
	case w.wake <- struct{}{}:
	default:
	}
}

// Run drives the worker until ctx is cancelled. It drains all claimable work on
// each wake or tick, so a burst of submissions is handled without waiting for the
// next tick.
func (w *Worker) Run(ctx context.Context) {
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	for {
		w.drain(ctx)
		select {
		case <-ctx.Done():
			return
		case <-w.wake:
		case <-ticker.C:
		}
	}
}

// drain claims and executes operations until none are claimable or ctx ends.
func (w *Worker) drain(ctx context.Context) {
	for {
		if ctx.Err() != nil {
			return
		}
		op, ok, err := w.store.ClaimNextOperation(ctx)
		if err != nil {
			w.logger.Error("claim_failed", "error", err.Error())
			return
		}
		if !ok {
			return
		}
		w.execute(ctx, op)
	}
}

// execute runs one claimed operation through its lifecycle and records the
// outcome. It never panics out: a failure transitions the operation to Failed (or
// NeedsAttention via recovery) rather than crashing the worker.
func (w *Worker) execute(ctx context.Context, op platform.Operation) {
	w.event(ctx, op.ID, "running", "operation started")

	switch op.Type {
	case platform.OperationDelete:
		w.executeDelete(ctx, op)
	default:
		w.executeApply(ctx, op)
	}
}

// executeApply handles create/update/restart/reapply/rollback: freeze the
// execution spec, apply through Helm, observe, and record the result.
func (w *Worker) executeApply(ctx context.Context, op platform.Operation) {
	rev, err := w.store.GetRevision(ctx, op.TargetRevision)
	if err != nil {
		w.fail(ctx, op, "load-revision", err)
		return
	}

	frozen, err := w.freeze(ctx, op, rev)
	if err != nil {
		w.fail(ctx, op, "freeze", err)
		return
	}
	rev.Frozen = &frozen

	w.event(ctx, op.ID, "applying", "applying workload through Helm")
	status, err := w.helm.InstallOrUpgrade(ctx, op.AppID, rev)
	if err != nil {
		// A failed apply is recorded on the operation; recovery/restoration is M4.
		_ = w.store.MarkRevisionResult(ctx, rev.ID, platform.RevisionFailed)
		w.fail(ctx, op, "apply", err)
		return
	}
	w.event(ctx, op.ID, "applied", "helm release "+status.Status)

	w.observeBestEffort(ctx, op.AppID)

	if err := w.store.MarkRevisionResult(ctx, rev.ID, platform.RevisionSucceeded); err != nil {
		w.fail(ctx, op, "record", err)
		return
	}
	if err := w.store.TransitionOperation(ctx, op.ID, platform.OperationSucceeded, "succeeded", ""); err != nil {
		w.logger.Error("transition_failed", "operation", op.ID, "error", err.Error())
		return
	}
	w.event(ctx, op.ID, "succeeded", "operation completed")
}

func (w *Worker) executeDelete(ctx context.Context, op platform.Operation) {
	w.event(ctx, op.ID, "deleting", "removing workload and routes")
	if err := w.helm.Uninstall(ctx, op.AppID); err != nil {
		w.fail(ctx, op, "delete", err)
		return
	}
	if err := w.store.TransitionOperation(ctx, op.ID, platform.OperationSucceeded, "succeeded", ""); err != nil {
		w.logger.Error("transition_failed", "operation", op.ID, "error", err.Error())
		return
	}
	w.event(ctx, op.ID, "succeeded", "deletion completed; data retained")
}

// freeze produces the immutable execution spec and commits it before any Helm
// call. For a create or an image change the digest is resolved from the registry;
// for every other operation the already-recorded digest is reused, so a scale,
// restart, config change, or reapply can never silently change the running image.
// Freezing is one-shot in the store, so a retry reuses the same digest.
func (w *Worker) freeze(ctx context.Context, op platform.Operation, rev platform.Revision) (platform.FrozenSpec, error) {
	// If the revision was already frozen by a prior (interrupted) attempt, reuse it
	// verbatim — this is the core guarantee that a retry cannot change the image.
	if rev.Frozen != nil {
		return *rev.Frozen, nil
	}

	digest := rev.Spec.Image
	resolvesDigest := op.Type == platform.OperationCreate || op.Type == platform.OperationUpdate
	if resolvesDigest && w.inspector != nil {
		report, err := w.inspector.Inspect(ctx, rev.Spec.Image, rev.Spec.RegistryCredentialID)
		if err != nil {
			return platform.FrozenSpec{}, err
		}
		digest = pinDigest(rev.Spec.Image, report.Digest)
	}

	frozen := platform.FrozenSpec{
		ImageDigest: digest,
		FrozenAt:    w.clock.Now(),
	}
	if len(rev.Spec.SecretRefs) > 0 {
		frozen.SecretVersions = rev.Spec.SecretRefs
	}
	if err := w.store.FreezeExecutionSpec(ctx, rev.ID, frozen); err != nil {
		return platform.FrozenSpec{}, err
	}
	// Re-read so the returned spec reflects what was actually committed (the store
	// keeps the first freeze if one already existed).
	stored, err := w.store.GetRevision(ctx, rev.ID)
	if err != nil {
		return platform.FrozenSpec{}, err
	}
	if stored.Frozen == nil {
		return frozen, nil
	}
	return *stored.Frozen, nil
}

func (w *Worker) observeBestEffort(ctx context.Context, appID string) {
	if w.observer == nil {
		return
	}
	status, err := w.observer.Observe(ctx, appID)
	if err != nil {
		// Observation failure does not fail the operation; status is simply left
		// for the background observer to refresh.
		w.logger.Warn("observe_failed", "app", appID, "error", err.Error())
		return
	}
	_ = w.store.SaveObservedStatus(ctx, appID, status)
}

func (w *Worker) fail(ctx context.Context, op platform.Operation, stage string, cause error) {
	w.logger.Warn("operation_failed", "operation", op.ID, "stage", stage, "error", cause.Error())
	w.event(ctx, op.ID, stage, sanitize(cause))
	if err := w.store.TransitionOperation(ctx, op.ID, platform.OperationFailed, stage, ""); err != nil {
		w.logger.Error("transition_failed", "operation", op.ID, "error", err.Error())
	}
}

func (w *Worker) event(ctx context.Context, opID, stage, message string) {
	_ = w.store.AppendOperationEvent(ctx, platform.OperationEvent{
		OperationID: opID, At: w.clock.Now(), Stage: stage, Message: message,
	})
}

// sanitize returns a message safe for operation history. Domain errors carry a
// vetted message; other errors are reported generically to avoid leaking detail.
func sanitize(err error) string {
	var derr *platform.Error
	if errors.As(err, &derr) {
		return derr.Message
	}
	return "operation failed"
}

// pinDigest returns an image reference pinned to digest. A reference that is
// already digest-pinned is returned unchanged; otherwise any tag is stripped and
// the digest appended.
func pinDigest(ref, digest string) string {
	for i := 0; i < len(ref); i++ {
		if ref[i] == '@' {
			return ref // already digest-pinned
		}
	}
	return stripTag(ref) + "@" + digest
}

// stripTag removes a trailing :tag from an image reference, preserving a
// registry:port host. It returns the repository without tag or digest.
func stripTag(ref string) string {
	lastColon := -1
	lastSlash := -1
	for i := 0; i < len(ref); i++ {
		switch ref[i] {
		case ':':
			lastColon = i
		case '/':
			lastSlash = i
		case '@':
			return ref[:i]
		}
	}
	// A ':' after the final '/' is a tag separator; before it (or absent) it is a
	// registry port and there is no tag to strip.
	if lastColon > lastSlash {
		return ref[:lastColon]
	}
	return ref
}
