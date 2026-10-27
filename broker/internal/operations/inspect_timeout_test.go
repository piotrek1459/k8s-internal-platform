package operations

import (
	"context"
	"testing"
	"time"

	"k8s-internal-platform/broker/internal/platform"
)

// hangingInspector blocks until its inspect context is cancelled, modelling a
// registry that accepts the connection and then stops responding.
type hangingInspector struct{}

func (hangingInspector) Inspect(ctx context.Context, _, _ string) (platform.ImageReport, error) {
	<-ctx.Done()
	return platform.ImageReport{}, ctx.Err()
}

func TestWorkerStalledRegistryDoesNotBlockQueue(t *testing.T) {
	st := newStore(t)
	helm := &fakeHelm{}
	w := NewWorker(st, helm, nil, hangingInspector{}, fixedClock{t: time.Unix(1, 0).UTC()}, discard())
	w.SetInspectTimeout(200 * time.Millisecond)

	// The first create triggers inspection, which hangs until the budget expires.
	stallOp, _ := seedCreate(t, st, "stalled", "ghcr.io/example/stalled:latest")
	// A second queued create must still be reached and processed.
	okOp, _ := seedCreate(t, st, "healthy", "ghcr.io/example/healthy:1.0")

	done := make(chan struct{})
	start := time.Now()
	go func() { w.drain(context.Background()); close(done) }()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("worker did not drain within 10s; a stalled registry blocked the queue")
	}
	elapsed := time.Since(start)

	// The stalled op fails (pre-mutation) within its budget; the lock is released
	// using the still-live store context, not the cancelled inspection context.
	so, _ := st.GetOperation(context.Background(), stallOp)
	if so.State != platform.OperationFailed {
		t.Fatalf("stalled op state = %s, want failed", so.State)
	}
	// The worker went on to the second operation rather than stranding it.
	oo, _ := st.GetOperation(context.Background(), okOp)
	if oo.State == platform.OperationQueued {
		t.Fatal("second op was never processed (still queued); queue was blocked")
	}
	if elapsed > 5*time.Second {
		t.Fatalf("draining took %v; inspection budget not enforced", elapsed)
	}
}
