package platform

import (
	"errors"
	"strings"
	"testing"
)

func TestOperationStateIsTerminal(t *testing.T) {
	terminal := map[OperationState]bool{
		OperationQueued:         false,
		OperationRunning:        false,
		OperationRecovering:     false,
		OperationNeedsAttention: false,
		OperationSucceeded:      true,
		OperationFailed:         true,
	}
	for state, want := range terminal {
		if got := state.IsTerminal(); got != want {
			t.Errorf("%s.IsTerminal() = %v, want %v", state, got, want)
		}
	}
}

func TestRandomIDGeneratorProducesUniquePrefixedIDs(t *testing.T) {
	var g RandomIDGenerator
	seen := make(map[string]bool)
	for range 1000 {
		id := g.NewID("op")
		if !strings.HasPrefix(id, "op_") {
			t.Fatalf("id %q missing prefix", id)
		}
		if seen[id] {
			t.Fatalf("duplicate id %q", id)
		}
		seen[id] = true
	}
}

func TestErrorFormatting(t *testing.T) {
	plain := Errorf(CodeNotFound, "application %s missing", "app_1")
	if got := plain.Error(); !strings.Contains(got, "NOT_FOUND") || !strings.Contains(got, "app_1") {
		t.Fatalf("unexpected message: %q", got)
	}
	withField := FieldErrorf(CodeInvalidInput, "port", "out of range")
	if !strings.Contains(withField.Error(), "field port") {
		t.Fatalf("field not reported: %q", withField.Error())
	}
}

func TestErrorUnwrap(t *testing.T) {
	cause := errors.New("disk full")
	derr := &Error{Code: CodeInternal, Message: "write failed", Err: cause}
	if !errors.Is(derr, cause) {
		t.Fatal("errors.Is did not find the wrapped cause")
	}
	var target *Error
	if !errors.As(derr, &target) {
		t.Fatal("errors.As did not match *Error")
	}
}
