package cluster

import (
	"context"
	"encoding/base64"
	"errors"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	kt "k8s.io/client-go/testing"
)

func TestConfigurationSecretErrorDoesNotExposeAdmissionResponse(t *testing.T) {
	value := "controlled-secret-error-value"
	encoded := base64.StdEncoding.EncodeToString([]byte(value))
	cs := fake.NewSimpleClientset()
	cs.PrependReactor("create", "secrets", func(kt.Action) (bool, runtime.Object, error) { return true, nil, errors.New(value + " " + encoded) })
	_, _, err := NewWithClientset(cs, ns).CreateImmutableSecret(context.Background(), "controlled", 1, map[string][]byte{"value": []byte(value)})
	if err == nil || strings.Contains(err.Error(), value) || strings.Contains(err.Error(), encoded) {
		t.Fatal("external Secret error was not safely classified")
	}
}
