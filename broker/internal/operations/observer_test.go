package operations

import (
	"context"
	"errors"
	"testing"
	"time"

	"k8s-internal-platform/broker/internal/platform"
)

// scriptedObserver returns a sequence of results, one per Observe call.
type scriptedObserver struct {
	calls int
	steps []struct {
		status platform.ObservedStatus
		err    error
	}
}

func (s *scriptedObserver) Observe(context.Context, string) (platform.ObservedStatus, error) {
	i := s.calls
	if i >= len(s.steps) {
		i = len(s.steps) - 1
	}
	s.calls++
	return s.steps[i].status, s.steps[i].err
}

func TestObserverMarksStaleOnReadFailureAndPreservesTimestamp(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	// Seed one application via a create so ListApplications returns it.
	seedCreate(t, st, "app1", "ghcr.io/example/app:1.0")

	fresh := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	scripted := &scriptedObserver{steps: []struct {
		status platform.ObservedStatus
		err    error
	}{
		{status: platform.ObservedStatus{Phase: platform.WorkloadReady, WorkloadReady: true, ObservedAt: fresh}},
		{err: errors.New("api server unreachable")},
	}}
	o := NewObserver(st, scripted, fixedClock{t: fresh.Add(time.Hour)}, discard())

	// First reconcile records a fresh ready status.
	o.reconcile(ctx, "app1")
	got, err := st.GetObservedStatus(ctx, "app1")
	if err != nil {
		t.Fatalf("get status: %v", err)
	}
	if got.Stale || !got.WorkloadReady || !got.ObservedAt.Equal(fresh) {
		t.Fatalf("after fresh read: %+v", got)
	}

	// Second reconcile fails: the previous observation and its timestamp are
	// preserved, and the status is marked stale rather than overwritten as fresh.
	o.reconcile(ctx, "app1")
	got, err = st.GetObservedStatus(ctx, "app1")
	if err != nil {
		t.Fatalf("get status: %v", err)
	}
	if !got.Stale {
		t.Fatal("status not marked stale after a failed read")
	}
	if !got.ObservedAt.Equal(fresh) {
		t.Fatalf("stale observation did not preserve the last successful timestamp: %v", got.ObservedAt)
	}
	if !got.WorkloadReady {
		t.Fatal("previous ready observation was lost on a failed read")
	}
}

func TestObserverReconcileAllCoversListedApps(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	seedCreate(t, st, "app1", "ghcr.io/example/app:1.0")

	scripted := &scriptedObserver{steps: []struct {
		status platform.ObservedStatus
		err    error
	}{
		{status: platform.ObservedStatus{Phase: platform.WorkloadReady, WorkloadReady: true, ObservedAt: time.Now().UTC()}},
	}}
	o := NewObserver(st, scripted, fixedClock{t: time.Now().UTC()}, discard())
	o.reconcileAll(ctx)
	if scripted.calls == 0 {
		t.Fatal("reconcileAll did not observe the listed application")
	}
}
