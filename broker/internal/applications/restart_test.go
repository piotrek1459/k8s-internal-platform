package applications

import (
	"k8s-internal-platform/broker/internal/platform"
	"testing"
)

func TestRestartCreatesNewSnapshotWithoutChangingImage(t *testing.T) {
	f := Factory{IDs: platform.RandomIDGenerator{}, Clock: platform.SystemClock{}}
	original := platform.Revision{ID: "old", Spec: platform.RevisionSpec{Image: "example.com/app:mutable"}, Frozen: &platform.FrozenSpec{ImageDigest: "example.com/app@sha256:original", RestartToken: "previous"}}
	req := f.NewRestartRequest(platform.Application{ID: "app", Version: 1}, original, 1, "restart", "hash", "admin", "")
	if req.Revision.Frozen != nil {
		t.Fatal("new restart reused old immutable snapshot")
	}
	if req.Revision.Spec.Image != original.Frozen.ImageDigest {
		t.Fatal("restart would resolve a mutable tag")
	}
	if original.Frozen.RestartToken != "previous" {
		t.Fatal("restart mutated previous snapshot")
	}
}
