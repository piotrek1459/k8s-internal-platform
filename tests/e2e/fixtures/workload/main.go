// This workload belongs only to disposable E2E clusters. It is not included in
// platform releases. Readiness is released explicitly; outbound probes report
// status/transport classification without retaining response or credential data.
package main

import (
	"context"
	"crypto/subtle"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"time"
)

var version = "fixture-dev"

type fixture struct {
	ready        atomic.Bool
	controlToken string
	platformURL  string
	kubeURL      string
	kubeCA       string
}

type probeResult struct {
	Name           string `json:"name"`
	Status         int    `json:"status"`
	TransportError bool   `json:"transportError"`
	Kind           string `json:"kind,omitempty"`
	Reason         string `json:"reason,omitempty"`
}

func (f *fixture) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, _ *http.Request) {
		if !f.ready.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("GET /version", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]string{"version": version})
	})
	mux.HandleFunc("POST /control/ready", func(w http.ResponseWriter, req *http.Request) {
		if f.controlToken == "" || subtle.ConstantTimeCompare([]byte(req.Header.Get("X-Fixture-Control")), []byte(f.controlToken)) != 1 {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		f.ready.Store(true)
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /probe", f.probe)
	return mux
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(value)
}

// request deliberately creates no cookie jar and never loads ambient credentials.
// Body contents are discarded after extracting only Kubernetes Status metadata.
func request(ctx context.Context, client *http.Client, name, method, url, body string) probeResult {
	result := probeResult{Name: name}
	req, err := http.NewRequestWithContext(ctx, method, url, strings.NewReader(body))
	if err != nil {
		result.TransportError = true
		return result
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	req.Header.Set("Idempotency-Key", "unauthenticated-workload-probe")
	response, err := client.Do(req)
	if err != nil {
		result.TransportError = true
		return result
	}
	defer response.Body.Close()
	result.Status = response.StatusCode
	if name == "kube-read" || name == "kube-create" {
		var status struct {
			Kind   string `json:"kind"`
			Reason string `json:"reason"`
		}
		if json.NewDecoder(io.LimitReader(response.Body, 4096)).Decode(&status) == nil {
			// Return only known classifications, never arbitrary server text.
			if status.Kind == "Status" {
				result.Kind = status.Kind
			}
			if status.Reason == "Unauthorized" || status.Reason == "Forbidden" {
				result.Reason = status.Reason
			}
		}
	}
	return result
}

func (f *fixture) probe(w http.ResponseWriter, req *http.Request) {
	if f.platformURL == "" || f.kubeURL == "" {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM([]byte(f.kubeCA)) || !strings.HasPrefix(f.kubeURL, "https://") {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	transport := &http.Transport{TLSClientConfig: &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12}}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 5 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	ctx, cancel := context.WithTimeout(req.Context(), 25*time.Second)
	defer cancel()
	results := []probeResult{
		request(ctx, client, "platform-health", "GET", f.platformURL+"/healthz", ""),
		request(ctx, client, "platform-read", "GET", f.platformURL+"/api/v1/apps", ""),
		request(ctx, client, "platform-create", "POST", f.platformURL+"/api/v1/apps", `{"name":"workload-boundary-attempt","image":"invalid","containerPort":8080,"profile":"http","exposure":{"mode":"internal"}}`),
		request(ctx, client, "platform-secret", "POST", f.platformURL+"/api/v1/secrets", `{"name":"workload-boundary-attempt","data":{"value":"controlled-fixture"}}`),
		request(ctx, client, "kube-read", "GET", f.kubeURL+"/api/v1/namespaces/platform-system/secrets?limit=1", ""),
		request(ctx, client, "kube-create", "POST", f.kubeURL+"/api/v1/namespaces", `{"apiVersion":"v1","kind":"Namespace","metadata":{"name":"workload-boundary-attempt"}}`),
	}
	_, tokenErr := os.Stat("/var/run/secrets/kubernetes.io/serviceaccount/token")
	writeJSON(w, map[string]any{"serviceAccountTokenAbsent": os.IsNotExist(tokenErr), "requests": results})
}

func main() {
	f := &fixture{controlToken: os.Getenv("FIXTURE_CONTROL_TOKEN"), platformURL: os.Getenv("FIXTURE_PLATFORM_URL"),
		kubeURL: os.Getenv("FIXTURE_KUBE_URL"), kubeCA: os.Getenv("FIXTURE_KUBE_CA")}
	f.ready.Store(os.Getenv("FIXTURE_START_READY") == "true")
	server := &http.Server{Addr: ":8080", Handler: f.handler(), ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout: 10 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 30 * time.Second}
	if err := server.ListenAndServe(); err != nil {
		fmt.Fprintln(os.Stderr, "fixture server stopped")
		os.Exit(1)
	}
}
