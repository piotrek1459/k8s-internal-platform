package operations

import (
	"context"
	"log/slog"
	"time"

	"k8s-internal-platform/broker/internal/platform"
)

// observeInterval is how often the background observer reconciles status for all
// managed applications.
const observeInterval = 15 * time.Second

// Observer periodically reconciles the observed status of every managed
// application. It runs independently of the operation worker so status stays
// current after deployments complete, after Pods disappear without a new
// operation, and across broker restarts. When a cluster read fails it marks the
// cached status stale while preserving the last successful observation timestamp,
// rather than overwriting it with a fresh-looking value.
type Observer struct {
	store    platform.Store
	observer platform.ClusterObserver
	clock    platform.Clock
	logger   *slog.Logger
	interval time.Duration
}

// NewObserver builds a background observer.
func NewObserver(store platform.Store, observer platform.ClusterObserver, clock platform.Clock, logger *slog.Logger) *Observer {
	if logger == nil {
		logger = slog.Default()
	}
	return &Observer{store: store, observer: observer, clock: clock, logger: logger, interval: observeInterval}
}

// SetInterval overrides the reconcile interval (used by tests).
func (o *Observer) SetInterval(d time.Duration) { o.interval = d }

// Run reconciles on a ticker until ctx is cancelled. It reconciles once
// immediately so status is refreshed promptly after a restart.
func (o *Observer) Run(ctx context.Context) {
	ticker := time.NewTicker(o.interval)
	defer ticker.Stop()
	for {
		o.reconcileAll(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// reconcileAll refreshes status for every non-deleted application.
func (o *Observer) reconcileAll(ctx context.Context) {
	apps, err := o.store.ListApplications(ctx, 1000, 0)
	if err != nil {
		o.logger.Warn("observer_list_failed", "error", err.Error())
		return
	}
	for _, app := range apps {
		o.reconcile(ctx, app.ID)
	}
}

// reconcile refreshes one application's status. On a read failure it preserves
// the last successful observation and marks it stale, so a stale ready status is
// never presented as fresh and a later observation can clear the flag.
func (o *Observer) reconcile(ctx context.Context, appID string) {
	status, err := o.observer.Observe(ctx, appID)
	if err == nil {
		status.Stale = false
		_ = o.store.SaveObservedStatus(ctx, appID, status)
		return
	}
	// Read failed: keep the previous observation (including its ObservedAt) and
	// mark it stale. If there is no prior observation, record an explicit stale
	// unknown so a client still sees that the state is not confirmed.
	prev, perr := o.store.GetObservedStatus(ctx, appID)
	if perr != nil {
		prev = platform.ObservedStatus{Phase: platform.WorkloadUnknown, ObservedAt: o.clock.Now()}
	}
	prev.Stale = true
	_ = o.store.SaveObservedStatus(ctx, appID, prev)
}
