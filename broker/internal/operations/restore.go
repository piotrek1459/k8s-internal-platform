package operations

import (
	"context"
	"k8s-internal-platform/broker/internal/platform"
)

// restore keeps the failed operation locked until the previous successful
// snapshot is reapplied and both release identity and live health are verified.
func (w *Worker) restore(ctx context.Context, op platform.Operation) bool {
	verifier, ok := w.helm.(platform.ReleaseVerifier)
	if !ok {
		return false
	}
	app, err := w.store.GetApplication(ctx, op.AppID)
	if err != nil || app.LastSuccessfulRevision == "" {
		return false
	}
	previous, err := w.store.GetRevision(ctx, app.LastSuccessfulRevision)
	if err != nil || previous.Frozen == nil || previous.Frozen.ChartArtifactDigest == "" {
		return false
	}
	if err := w.store.TransitionOperation(ctx, op.ID, platform.OperationRecovering, "restoring", ""); err != nil {
		return false
	}
	w.event(ctx, op.ID, "restoring", "restoring the previous successful revision")
	if _, err := w.helm.InstallOrUpgrade(ctx, op.AppID, previous); err != nil {
		return false
	}
	if err := verifier.VerifyTarget(ctx, op.AppID, previous); err != nil {
		return false
	}
	if err := w.store.RestoreApplication(ctx, op.ID, previous.ID); err != nil {
		return false
	}
	w.observeBestEffort(ctx, op.AppID)
	w.event(ctx, op.ID, "restored", "candidate failed; previous successful revision restored and verified")
	return true
}
