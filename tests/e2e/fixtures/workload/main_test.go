package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestReadinessRequiresAuthenticatedExplicitRelease(t *testing.T) {
	f := &fixture{controlToken: "controlled-gate-token"}
	h := f.handler()
	check := func(method, path, token string, expected int) {
		t.Helper()
		req := httptest.NewRequest(method, path, nil)
		req.Header.Set("X-Fixture-Control", token)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != expected {
			t.Fatalf("%s %s: status %d, expected %d", method, path, rec.Code, expected)
		}
	}
	check("GET", "/healthz", "", 200)
	check("GET", "/readyz", "", 503)
	check("POST", "/control/ready", "incorrect", 403)
	check("GET", "/readyz", "", 503)
	check("POST", "/control/ready", f.controlToken, 204)
	check("GET", "/readyz", "", 200)
}

func TestProbeDoesNotFollowRedirectOrReturnResponseContent(t *testing.T) {
	var called atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path == "/redirect" {
			w.Header().Set("Location", "/sensitive")
			w.WriteHeader(302)
			return
		}
		called.Store(true)
		_, _ = w.Write([]byte(`{"kind":"Status","reason":"controlled-private-response"}`))
	}))
	defer server.Close()
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	result := request(context.Background(), client, "kube-read", "GET", server.URL+"/redirect", "")
	if result.Status != 302 || result.TransportError || called.Load() {
		t.Fatal("probe followed a redirect or reported a false transport failure")
	}
	result = request(context.Background(), client, "kube-read", "GET", server.URL+"/sensitive", "")
	if !called.Load() || result.Kind != "Status" || result.Reason != "" {
		t.Fatal("probe returned arbitrary response text")
	}
}

func TestTransportFailureCannotMasqueradeAsAuthorizationRefusal(t *testing.T) {
	result := request(context.Background(), &http.Client{}, "kube-read", "GET", ":invalid", "")
	if !result.TransportError || result.Status != 0 || result.Reason != "" {
		t.Fatal("transport failure was classified as an authorization response")
	}
	f := &fixture{platformURL: "http://platform", kubeURL: "https://kubernetes", kubeCA: "invalid"}
	rec := httptest.NewRecorder()
	f.handler().ServeHTTP(rec, httptest.NewRequest("POST", "/probe", strings.NewReader("")))
	if rec.Code != 503 {
		t.Fatal("probe allowed unverified Kubernetes TLS")
	}
}
