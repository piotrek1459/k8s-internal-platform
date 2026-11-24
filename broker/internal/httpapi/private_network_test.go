package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"k8s-internal-platform/broker/internal/applications"
	"k8s-internal-platform/broker/internal/platform"
)

func testPrivateNetwork() PrivateNetwork {
	return PrivateNetwork{ManagementHostname: "platform.home.arpa", ApplicationDomain: "apps.home.arpa", HTTPSPort: 8443}
}

func TestPrivateNetworkRejectsInvalidOrOverlappingConfiguration(t *testing.T) {
	valid := testPrivateNetwork()
	if err := valid.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*PrivateNetwork){
		func(n *PrivateNetwork) { n.ManagementHostname = "platform.apps.home.arpa" },
		func(n *PrivateNetwork) { n.ManagementHostname = n.ApplicationDomain },
		func(n *PrivateNetwork) { n.ApplicationDomain = "apps..home.arpa" },
		func(n *PrivateNetwork) { n.ApplicationDomain = "apps.HOME.arpa" },
		func(n *PrivateNetwork) { n.ManagementHostname = strings.Repeat("a", 64) + ".home.arpa" },
		func(n *PrivateNetwork) { n.HTTPSPort = 0 },
		func(n *PrivateNetwork) { n.HTTPSPort = 65536 },
	} {
		candidate := valid
		change(&candidate)
		if err := candidate.Validate(); err == nil {
			t.Fatalf("accepted invalid configuration: %+v", candidate)
		}
	}
}

func TestPrivateExposureRefusesUnconfiguredAndReservedRoutes(t *testing.T) {
	router := NewRouter(Config{PrivateIngressEnabled: true, PrivateNetwork: testPrivateNetwork()})
	for _, host := range []string{"platform.home.arpa", "apps.home.arpa", "deep.demo.apps.home.arpa", "demo.apps.home.arpa.evil", "elsewhere.example"} {
		if err := router.validatePrivateExposure(platform.Exposure{Mode: platform.ExposureLAN, Hostname: host}); err == nil {
			t.Fatalf("accepted host %s", host)
		}
	}
	if err := router.validatePrivateExposure(platform.Exposure{Mode: platform.ExposureLAN, Hostname: "demo.apps.home.arpa"}); err != nil {
		t.Fatal(err)
	}
	router.cfg.PrivateIngressEnabled = false
	if err := router.validatePrivateExposure(platform.Exposure{Mode: platform.ExposureLAN, Hostname: "demo.apps.home.arpa"}); err == nil {
		t.Fatal("accepted unconfigured ingress")
	}
	if err := router.validatePrivateExposure(platform.Exposure{Mode: platform.ExposureInternal}); err != nil {
		t.Fatal(err)
	}
}

type transitionStore struct {
	*fakeStore
	revision platform.Revision
}

func (s *transitionStore) GetApplicationWithRevision(ctx context.Context, id string) (platform.Application, platform.Revision, error) {
	app, err := s.GetApplication(ctx, id)
	return app, s.revision, err
}
func TestPrivateAccessTransitionsUseTrackedAcceptanceAndHostnameValidation(t *testing.T) {
	internal := platform.Exposure{Mode: platform.ExposureInternal}
	lan := platform.Exposure{Mode: platform.ExposureLAN, Hostname: "demo.apps.home.arpa"}
	renamed := platform.Exposure{Mode: platform.ExposureLAN, Hostname: "renamed.apps.home.arpa"}
	for _, pair := range [][2]platform.Exposure{{internal, lan}, {lan, internal}, {lan, renamed}} {
		st := &transitionStore{fakeStore: newFakeStore(), revision: platform.Revision{ID: "old", Spec: platform.RevisionSpec{Image: "example.com/app@sha256:" + strings.Repeat("a", 64), ContainerPort: 8080, Replicas: 1, Profile: platform.ProfileHTTP, Exposure: pair[0]}}}
		st.apps["app"] = platform.Application{ID: "app", Name: "tracked", Version: 1}
		calls := 0
		st.accept = func(_ context.Context, req platform.AcceptRequest) (platform.AcceptResult, error) {
			calls++
			if req.Revision.Spec.Exposure != pair[1] || req.ExpectVersion != 1 {
				t.Fatal("transition lost desired exposure or version guard")
			}
			return platform.AcceptResult{ApplicationID: "app", OperationID: "op", State: platform.OperationQueued}, nil
		}
		h := NewRouter(Config{Store: st, Factory: applications.Factory{IDs: platform.RandomIDGenerator{}, Clock: platform.SystemClock{}}, PrivateIngressEnabled: true, PrivateNetwork: testPrivateNetwork()}).Handler()
		body, _ := json.Marshal(map[string]any{"version": 1, "exposure": map[string]string{"mode": string(pair[1].Mode), "hostname": pair[1].Hostname}})
		req := httptest.NewRequest(http.MethodPatch, "/api/v1/apps/app", strings.NewReader(string(body)))
		req.Header.Set("Idempotency-Key", "controlled-transition")
		req.Header.Set("X-Requested-With", "XMLHttpRequest")
		response := httptest.NewRecorder()
		h.ServeHTTP(response, req)
		if response.Code != 202 || calls != 1 {
			t.Fatalf("tracked transition status=%d calls=%d body=%s", response.Code, calls, response.Body.String())
		}
		bad := httptest.NewRequest(http.MethodPatch, "/api/v1/apps/app", strings.NewReader(`{"version":1,"exposure":{"mode":"lan","hostname":"outside.example"}}`))
		bad.Header.Set("Idempotency-Key", "invalid-transition")
		bad.Header.Set("X-Requested-With", "XMLHttpRequest")
		response = httptest.NewRecorder()
		h.ServeHTTP(response, bad)
		if response.Code != 400 || calls != 1 {
			t.Fatal("out-of-domain hostname reached acceptance")
		}
	}
}

type accessStore struct {
	*fakeStore
	revisions map[string]platform.Revision
}

func (s *accessStore) GetRevision(_ context.Context, id string) (platform.Revision, error) {
	return s.revisions[id], nil
}

func TestApplicationAccessUsesSuccessfulRevisionUntilCandidateSucceeds(t *testing.T) {
	st := &accessStore{fakeStore: newFakeStore(), revisions: map[string]platform.Revision{
		"successful": {ID: "successful", Spec: platform.RevisionSpec{Exposure: platform.Exposure{Mode: platform.ExposureLAN, Hostname: "old.apps.home.arpa"}}},
		"candidate":  {ID: "candidate", Spec: platform.RevisionSpec{Exposure: platform.Exposure{Mode: platform.ExposureLAN, Hostname: "new.apps.home.arpa"}}},
	}}
	st.apps["app"] = platform.Application{ID: "app", DesiredRevisionID: "candidate", LastSuccessfulRevision: "successful"}
	h := NewRouter(Config{Store: st, PrivateNetwork: testPrivateNetwork()}).Handler()
	for _, successful := range []bool{false, true} {
		if successful {
			app := st.apps["app"]
			app.LastSuccessfulRevision = "candidate"
			st.apps["app"] = app
		}
		response := httptest.NewRecorder()
		h.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/apps/app", nil))
		if response.Code != 200 {
			t.Fatalf("status %d: %s", response.Code, response.Body.String())
		}
		var body struct {
			Access struct {
				URL        string `json:"url"`
				RevisionID string `json:"revisionId"`
			} `json:"access"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		expected := "https://old.apps.home.arpa:8443/"
		revision := "successful"
		if successful {
			expected = "https://new.apps.home.arpa:8443/"
			revision = "candidate"
		}
		if body.Access.URL != expected || body.Access.RevisionID != revision {
			t.Fatalf("wrong access: %+v", body.Access)
		}
	}
}
