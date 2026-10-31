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
	"strings"
	"time"

	"github.com/google/go-containerregistry/pkg/name"

	"k8s-internal-platform/broker/internal/platform"
)

// pollInterval is the backstop cadence at which the worker looks for claimable
// work even without a wake signal, so operations recovered at startup drain and a
// missed wake never strands work.
const pollInterval = 5 * time.Second

// defaultInspectTimeout bounds registry inspection for a single operation. A
// registry that accepts the connection and then stalls must not block the serial
// worker indefinitely; dial/TLS timeouts alone do not cover that case.
const defaultInspectTimeout = 30 * time.Second

// Worker executes persisted operations serially. One Worker runs per broker
// process; the store's claim guarantees at most one operation per application and
// skips applications blocked by an unresolved NeedsAttention operation.
type Worker struct {
	store          platform.Store
	helm           platform.HelmInstaller
	observer       platform.ClusterObserver
	inspector      platform.ImageInspector
	clock          platform.Clock
	logger         *slog.Logger
	inspectTimeout time.Duration

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
		clock: clock, logger: logger, inspectTimeout: defaultInspectTimeout,
		wake: make(chan struct{}, 1),
	}
}

// SetInspectTimeout overrides the per-operation registry inspection budget. Used
// by tests to force a fast, deterministic timeout.
func (w *Worker) SetInspectTimeout(d time.Duration) { w.inspectTimeout = d }

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

// DrainOnce claims and executes all currently claimable operations, returning
// when none remain. It is exported for integration tests and tools that drive the
// worker synchronously; the running broker uses Run.
func (w *Worker) DrainOnce(ctx context.Context) { w.drain(ctx) }

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
		// The Helm call may have created or changed resources before failing (for
		// example a timed-out rollout). The cluster state is therefore unresolved,
		// so the failed candidate is recorded but the operation enters
		// NeedsAttention and retains the application lock rather than terminating.
		// Verified restoration to the previous revision is M4; until then a
		// documented recovery must resolve the block. (See the M2 completion
		// boundary in docs/milestones.md.)
		_ = w.store.MarkRevisionResult(ctx, rev.ID, platform.RevisionFailed)
		w.needsAttention(ctx, op, "apply", err)
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
		// A partial uninstall leaves the cluster in an unresolved state; retain the
		// lock for recovery rather than terminating.
		w.needsAttention(ctx, op, "delete", err)
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
	alreadyPinned := strings.Contains(rev.Spec.Image, "@sha256:")
	resolvesDigest := (op.Type == platform.OperationCreate || op.Type == platform.OperationUpdate) && !alreadyPinned
	if resolvesDigest && w.inspector != nil {
		// Bound the inspection with its own deadline so a registry that accepts the
		// connection and then stalls cannot block the serial worker. The outer ctx
		// (used by the caller to persist the outcome) is left untouched, so a
		// timeout here is still recorded on a live store context.
		inspectCtx, cancel := context.WithTimeout(ctx, w.inspectTimeout)
		report, err := w.inspector.Inspect(inspectCtx, rev.Spec.Image, rev.Spec.RegistryCredentialID)
		cancel()
		if err != nil {
			return platform.FrozenSpec{}, err
		}
		digest, err = pinDigest(rev.Spec.Image, report.Digest)
		if err != nil {
			return platform.FrozenSpec{}, err
		}
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

// fail records a terminal Failed outcome and releases the application lock. It is
// used only for failures that are known to precede any cluster mutation
// (validation, image inspection, freeze), where the working release is unchanged
// and no recovery is required.
func (w *Worker) fail(ctx context.Context, op platform.Operation, stage string, cause error) {
	w.logger.Warn("operation_failed", "operation", op.ID, "stage", stage, "error", cause.Error())
	w.event(ctx, op.ID, stage, sanitize(cause))
	if err := w.store.TransitionOperation(ctx, op.ID, platform.OperationFailed, stage, ""); err != nil {
		w.logger.Error("transition_failed", "operation", op.ID, "error", err.Error())
	}
}

// needsAttention records a nonterminal NeedsAttention outcome and retains the
// application lock. It is used when a failure may have left the cluster partially
// changed and the broker has not verified or restored a known-good state, so
// conflicting mutations must keep returning 409 until a documented recovery
// resolves it. Automatic restoration is M4.
func (w *Worker) needsAttention(ctx context.Context, op platform.Operation, stage string, cause error) {
	w.logger.Warn("operation_needs_attention", "operation", op.ID, "app", op.AppID, "stage", stage, "error", cause.Error())
	w.event(ctx, op.ID, "needs_attention", sanitize(cause))
	outcome := "unresolved after " + stage + "; manual recovery required"
	if err := w.store.TransitionOperation(ctx, op.ID, platform.OperationNeedsAttention, stage, outcome); err != nil {
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

// pinDigest returns an image reference pinned to the resolved digest. It parses
// the reference to recover the repository context (registry, port, repository
// path) and appends "@<digest>", discarding any tag or existing digest in the
// input. Using the registry name parser rather than hand-splitting handles
// registry ports, tag-plus-digest references, and digest-only references
// correctly, so the frozen reference is always a valid repository@digest form.
func pinDigest(ref, digest string) (string, error) {
	parsed, err := name.ParseReference(ref)
	if err != nil {
		return "", platform.FieldErrorf(platform.CodeInvalidInput, "image",
			"invalid image reference %q: %v", ref, err)
	}
	// Context().Name() is the fully qualified repository without tag or digest.
	return parsed.Context().Name() + "@" + digest, nil
}
