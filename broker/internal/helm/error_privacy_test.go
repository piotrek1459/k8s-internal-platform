package helm

import (
	"errors"
	"strings"
	"testing"
)

func TestHelmErrorsDoNotExposeAdmissionResponse(t *testing.T) {
	marker := "controlled-admission-secret-value"
	err := classifyHelmError(errors.New("admission rejected " + marker))
	if err == nil || strings.Contains(err.Error(), marker) {
		t.Fatal("Helm error exposed an untrusted diagnostic body")
	}
}
