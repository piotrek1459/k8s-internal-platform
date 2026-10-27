package operations

import (
	"context"
	"log/slog"

	"k8s-internal-platform/broker/internal/platform"
)

// Recovery reconciles operations left nonterminal by a process restart. It runs
// once at startup, before the worker begins serving, so an interrupted mutation
// is either safely resumed or durably blocked before any new work is accepted.
type Recovery struct {
	store  platform.Store
	helm   platform.HelmInstaller
	logger *slog.Logger
}

// NewRecovery builds a recovery engine.
func NewRecovery(store platform.Store, helm platform.HelmInstaller, logger *slog.Logger) *Recovery {
	if logger == nil {
		logger = slog.Default()
	}
	return &Recovery{store: store, helm: helm, logger: logger}
}

// Scan classifies every nonterminal operation. It never deletes Helm release
// records to clear a lock; an operation whose state cannot be safely established
// is moved to NeedsAttention, which retains the application's active-operation
// lock so conflicting mutations keep returning 409 until a documented recovery
// clears it.
func (r *Recovery) Scan(ctx context.Context) error {
	ops, err := r.store.ListNonterminalOperations(ctx)
	if err != nil {
		return err
	}
	for _, op := range ops {
		r.recoverOne(ctx, op)
	}
	return nil
}

func (r *Recovery) recoverOne(ctx context.Context, op platform.Operation) {
	switch op.State {
	case platform.OperationQueued:
		// Never started; the worker will pick it up normally. Nothing to recover.
		return
	case platform.OperationNeedsAttention:
		// Already blocked and awaiting operator action; leave it untouched.
		return
	case platform.OperationRunning, platform.OperationRecovering:
		r.recoverInterrupted(ctx, op)
	}
}

// recoverInterrupted handles an operation that was mid-flight when the process
// stopped. It inspects the Helm release to decide whether the work completed,
// can be safely resumed, or is ambiguous.
func (r *Recovery) recoverInterrupted(ctx context.Context, op platform.Operation) {
	r.event(ctx, op.ID, "recovering", "inspecting interrupted operation after restart")

	// A delete that was interrupted is safe to resume: Uninstall is idempotent and
	// deletion has no digest to re-resolve. Re-queue it as Recovering.
	if op.Type == platform.OperationDelete {
		r.resume(ctx, op, "resuming interrupted deletion")
		return
	}

	rev, err := r.store.GetRevision(ctx, op.TargetRevision)
	if err != nil {
		r.block(ctx, op, "cannot load target revision for recovery")
		return
	}

	status, exists, err := r.helm.Status(ctx, op.AppID)
	if err != nil {
		// The cluster could not be consulted; do not guess. Block until a later
		// documented recovery can establish the true state.
		r.block(ctx, op, "cluster state is unknown; cannot verify safe recovery")
		return
	}

	switch {
	case exists && status.Status == "deployed" && rev.Frozen != nil:
		// The target release is applied and healthy; finish the operation.
		r.finish(ctx, op, rev)
	case !exists && op.Type == platform.OperationCreate:
		// An interrupted initial install left no release at all, so re-executing is
		// safe: the frozen spec (if any) is reused, and there is no pending Helm
		// release to conflict with.
		r.resume(ctx, op, "resuming interrupted install (no release present)")
	case exists && (status.Status == "pending-install" || status.Status == "pending-upgrade" || status.Status == "pending-rollback"):
		// Helm recorded a pending release. Re-executing would make the SDK reject
		// the operation with "another operation is in progress", so this must not
		// be resumed automatically; block for a documented recovery instead.
		r.block(ctx, op, "a pending Helm release is in progress; manual recovery required before retry")
	default:
		// Anything else (failed release, ambiguous status) is not safe to resume
		// automatically.
		r.block(ctx, op, "release state is ambiguous; manual recovery required")
	}
}

// finish marks a verified-applied operation and revision successful and releases
// the application lock.
func (r *Recovery) finish(ctx context.Context, op platform.Operation, rev platform.Revision) {
	if err := r.store.MarkRevisionResult(ctx, rev.ID, platform.RevisionSucceeded); err != nil {
		r.logger.Error("recovery_mark_failed", "operation", op.ID, "error", err.Error())
		return
	}
	if err := r.store.TransitionOperation(ctx, op.ID, platform.OperationSucceeded, "recovered", "target already applied"); err != nil {
		r.logger.Error("recovery_transition_failed", "operation", op.ID, "error", err.Error())
		return
	}
	r.event(ctx, op.ID, "succeeded", "recovered: target was already applied")
}

// resume re-queues an operation as Recovering so the worker re-executes it,
// reusing the frozen spec. The application lock is retained throughout.
func (r *Recovery) resume(ctx context.Context, op platform.Operation, message string) {
	if err := r.store.TransitionOperation(ctx, op.ID, platform.OperationRecovering, "recovering", ""); err != nil {
		r.logger.Error("recovery_resume_failed", "operation", op.ID, "error", err.Error())
		return
	}
	r.event(ctx, op.ID, "recovering", message)
}

// block moves an operation to NeedsAttention, retaining the application lock so
// conflicting mutations keep returning 409 until a documented recovery verifies
// the resulting cluster state.
func (r *Recovery) block(ctx context.Context, op platform.Operation, reason string) {
	r.logger.Warn("operation_needs_attention", "operation", op.ID, "app", op.AppID, "reason", reason)
	if err := r.store.TransitionOperation(ctx, op.ID, platform.OperationNeedsAttention, "needs_attention", reason); err != nil {
		r.logger.Error("recovery_block_failed", "operation", op.ID, "error", err.Error())
		return
	}
	r.event(ctx, op.ID, "needs_attention", reason)
}

func (r *Recovery) event(ctx context.Context, opID, stage, message string) {
	_ = r.store.AppendOperationEvent(ctx, platform.OperationEvent{
		OperationID: opID, Stage: stage, Message: message,
	})
}
