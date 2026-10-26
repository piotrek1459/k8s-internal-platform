package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSystemReportsVersionAndCapabilities(t *testing.T) {
	h := NewRouter(Config{
		Logger:    discardLogger(),
		Version:   "1.2.3",
		Inspector: fakeInspector{},
	}).Handler()

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/system", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body struct {
		Version          string   `json:"version"`
		Architecture     string   `json:"architecture"`
		ClusterReachable bool     `json:"clusterReachable"`
		Capabilities     []string `json:"capabilities"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Version != "1.2.3" {
		t.Fatalf("version = %q", body.Version)
	}
	if body.Architecture == "" {
		t.Fatal("architecture not reported")
	}
	if body.ClusterReachable {
		t.Fatal("cluster should be unreachable without a pinger")
	}
	if !containsStr(body.Capabilities, "image-inspection") {
		t.Fatalf("capabilities = %v, want image-inspection", body.Capabilities)
	}
}

func containsStr(xs []string, want string) bool {
	for _, x := range xs {
		if x == want {
			return true
		}
	}
	return false
}
