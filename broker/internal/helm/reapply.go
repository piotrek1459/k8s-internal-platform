package helm

import (
	"context"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/storage/driver"

	"k8s-internal-platform/broker/internal/platform"
)

func (a *Adapter) WithMutationGuard(guard func(context.Context, string, string) error) *Adapter {
	a.guardMutation = guard
	return a
}

func (a *Adapter) checkMutation(ctx context.Context, cfg *action.Configuration, appID string, revision platform.Revision) error {
	if a.guardMutation == nil {
		return nil
	}
	target, err := a.RenderExecution(appID, revision)
	if err != nil {
		return err
	}
	current, err := action.NewGet(cfg).Run(releaseNameFor(appID))
	if err != nil && err != driver.ErrReleaseNotFound {
		return classifyHelmError(err)
	}
	if current != nil {
		target = current.Manifest + "\n---\n" + target
	}
	return a.guardMutation(ctx, appID, target)
}

func (a *Adapter) WithReapplyPreparer(prepare func(context.Context, string, string) error) *Adapter {
	a.prepareReapply = prepare
	return a
}

func (a *Adapter) WithRestorationPreparer(prepare func(context.Context, string, string) error) *Adapter {
	a.prepareRestoration = prepare
	return a
}

// PrepareRestoration refuses pending transactions before changing resources.
// Repair uses the recorded chart and values, never a substituted artifact.
func (a *Adapter) PrepareRestoration(ctx context.Context, appID string, revision platform.Revision) error {
	if a.prepareRestoration == nil {
		return nil
	}
	if _, err := a.PrepareExecution(appID, revision); err != nil {
		return err
	}
	status, exists, err := a.Status(ctx, appID)
	if err != nil {
		return err
	}
	if !exists || (status.Status != "deployed" && status.Status != "failed") {
		return platform.Errorf(platform.CodeConflict, "the release is not available for restoration; pending transactions require operator recovery")
	}
	cfg, err := a.actionConfigFor(a.namespace)
	if err != nil {
		return platform.Errorf(platform.CodeDependencyUnavailable, "could not initialize restoration")
	}
	if err := a.checkMutation(ctx, cfg, appID, revision); err != nil {
		return err
	}
	manifest, err := a.RenderExecution(appID, revision)
	if err != nil {
		return err
	}
	return a.prepareRestoration(ctx, appID, manifest)
}

func (a *Adapter) PrepareReapply(ctx context.Context, appID string, revision platform.Revision) error {
	if a.prepareReapply == nil {
		return nil
	}
	status, exists, err := a.Status(ctx, appID)
	if err != nil {
		return err
	}
	// Pending/unknown releases retain their ordinary recovery block; no repair
	// may mutate resources before Helm has rejected that unresolved transaction.
	if !exists {
		return nil
	}
	if status.Status != "deployed" && status.Status != "failed" {
		return platform.Errorf(platform.CodeConflict, "the release is not available for explicit reapply")
	}
	manifest, err := a.RenderExecution(appID, revision)
	if err != nil {
		return err
	}
	return a.prepareReapply(ctx, appID, manifest)
}
