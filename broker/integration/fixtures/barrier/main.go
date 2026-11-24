// This executable is an external E2E fixture, never part of platform releases.
// It gates Kubernetes admission and holds a write transaction without changing
// application data, establishing observable crash boundaries in an owned cluster.
package main

import (
	"context"
	"crypto/subtle"
	"crypto/tls"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	_ "modernc.org/sqlite"
)

type gateConfig struct {
	Mode    string `json:"mode"`
	Message string `json:"message"`
}
type fixture struct {
	token    string
	path     string
	conn     *sql.Conn
	mu       sync.Mutex
	config   gateConfig
	gate     chan struct{}
	hits     int
	waiting  int
	lastName string
	locked   bool
}

func write(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(value)
}
func (f *fixture) control(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if subtle.ConstantTimeCompare([]byte(r.Header.Get("X-Fixture-Control")), []byte(f.token)) != 1 {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		next(w, r)
	}
}
func (f *fixture) configure(w http.ResponseWriter, r *http.Request) {
	var cfg gateConfig
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&cfg) != nil || len(cfg.Message) > 1024 {
		w.WriteHeader(400)
		return
	}
	switch cfg.Mode {
	case "allow", "hold-create", "hold-deployed", "deny-secret", "deny-helm", "deny-deployment":
	default:
		w.WriteHeader(400)
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	close(f.gate)
	f.gate = make(chan struct{})
	f.config = cfg
	f.hits = 0
	f.lastName = ""
	w.WriteHeader(204)
}
func (f *fixture) admission(w http.ResponseWriter, r *http.Request) {
	var review admissionv1.AdmissionReview
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 2<<20)).Decode(&review) != nil || review.Request == nil {
		w.WriteHeader(400)
		return
	}
	req := review.Request
	response := &admissionv1.AdmissionResponse{UID: req.UID, Allowed: true}
	var secret corev1.Secret
	if req.Namespace == "platform-apps" && req.Resource.Group == "" && req.Resource.Resource == "secrets" && json.Unmarshal(req.Object.Raw, &secret) == nil {
		f.mu.Lock()
		cfg, gate := f.config, f.gate
		helm := secret.Labels["owner"] == "helm" && strings.HasPrefix(secret.Labels["name"], "app-")
		hold := helm && ((cfg.Mode == "hold-create" && req.Operation == admissionv1.Create && secret.Labels["status"] == "pending-install") || (cfg.Mode == "hold-deployed" && req.Operation == admissionv1.Update && secret.Labels["status"] == "deployed"))
		deny := req.Operation == admissionv1.Create && ((cfg.Mode == "deny-secret" && secret.Type == corev1.SecretTypeOpaque) || (cfg.Mode == "deny-helm" && helm))
		if hold || deny {
			f.hits++
			f.lastName = secret.Name
		}
		if hold {
			f.waiting++
		}
		f.mu.Unlock()
		if hold {
			timer := time.NewTimer(25 * time.Second)
			defer timer.Stop()
			select {
			case <-gate:
			case <-r.Context().Done():
			case <-timer.C:
			}
			f.mu.Lock()
			f.waiting--
			f.mu.Unlock()
			// A request already held at the crash boundary is refused when released.
			// Only new requests see the next mode, so a dead broker cannot commit late.
			response.Allowed = false
			response.Result = &metav1.Status{Message: "controlled admission boundary released", Code: 403}
		} else if deny {
			response.Allowed = false
			response.Result = &metav1.Status{Message: cfg.Message, Code: 403}
		}
	}
	if req.Namespace == "platform-apps" && req.Resource.Group == "apps" && req.Resource.Resource == "deployments" {
		f.mu.Lock()
		if f.config.Mode == "deny-deployment" {
			f.hits++
			f.lastName = req.Name
			response.Allowed = false
			response.Result = &metav1.Status{Message: f.config.Message, Code: 403}
		}
		f.mu.Unlock()
	}

	write(w, admissionv1.AdmissionReview{TypeMeta: metav1.TypeMeta{APIVersion: "admission.k8s.io/v1", Kind: "AdmissionReview"}, Response: response})
}
func (f *fixture) status(w http.ResponseWriter, _ *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	write(w, map[string]any{"hits": f.hits, "waiting": f.waiting, "lastName": f.lastName, "locked": f.locked})
}
func (f *fixture) lock(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Held bool `json:"held"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&input) != nil {
		w.WriteHeader(400)
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if input.Held == f.locked {
		w.WriteHeader(204)
		return
	}
	statement := "BEGIN IMMEDIATE"
	if !input.Held {
		statement = "ROLLBACK"
	}
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	if _, err := f.conn.ExecContext(ctx, statement); err != nil {
		w.WriteHeader(409)
		return
	}
	f.locked = input.Held
	w.WriteHeader(204)
}
func (f *fixture) state(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Operation string `json:"operation"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&input) != nil || len(input.Operation) > 128 {
		w.WriteHeader(400)
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	var state, stage, revision, result, successful, active string
	var image, chartVersion, chart, values, refs, frozenAt, restart string
	err := f.conn.QueryRowContext(r.Context(), `SELECT o.state,o.stage,o.target_revision,r.result,COALESCE(a.last_successful_revision,''),COALESCE(a.active_operation_id,''),COALESCE(r.image_digest,''),COALESCE(r.chart_version,''),COALESCE(r.chart_artifact_digest,''),COALESCE(r.values_hash,''),COALESCE(r.secret_versions_json,'[]'),COALESCE(r.frozen_at,''),COALESCE(r.restart_token,'') FROM operations o JOIN revisions r ON r.id=o.target_revision JOIN applications a ON a.id=o.app_id WHERE o.id=?`, input.Operation).Scan(&state, &stage, &revision, &result, &successful, &active, &image, &chartVersion, &chart, &values, &refs, &frozenAt, &restart)
	if err != nil {
		w.WriteHeader(404)
		return
	}
	write(w, map[string]any{"state": state, "stage": stage, "targetRevision": revision, "result": result, "lastSuccessfulRevision": successful, "activeOperation": active, "frozen": map[string]string{"image": image, "chartVersion": chartVersion, "chart": chart, "values": values, "secretRefs": refs, "frozenAt": frozenAt, "restart": restart}})
}
func (f *fixture) scan(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Needles  []string `json:"needles"`
		Positive string   `json:"positive"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 128<<10)).Decode(&input) != nil || len(input.Needles) == 0 || len(input.Needles) > 64 || input.Positive == "" {
		w.WriteHeader(400)
		return
	}
	for _, n := range append(input.Needles, input.Positive) {
		if len(n) < 8 || len(n) > 4096 {
			w.WriteHeader(400)
			return
		}
	}
	found, positive, files := false, false, 0
	for _, name := range []string{f.path, f.path + "-wal", f.path + "-shm"} {
		file, err := os.Open(name)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			w.WriteHeader(503)
			return
		}
		data, err := io.ReadAll(io.LimitReader(file, (64<<20)+1))
		_ = file.Close()
		if err != nil || len(data) > 64<<20 {
			w.WriteHeader(503)
			return
		}
		files++
		text := string(data)
		positive = positive || strings.Contains(text, input.Positive)
		for _, n := range input.Needles {
			found = found || strings.Contains(text, n)
		}
	}
	write(w, map[string]any{"found": found, "positiveFound": positive, "files": files})
}
func main() {
	token := os.Getenv("FIXTURE_CONTROL_TOKEN")
	cluster := os.Getenv("FIXTURE_OWNED_CLUSTER")
	path := "/var/lib/platform/platform.db"
	if len(token) < 32 || !strings.HasPrefix(cluster, "platform-e2e-") {
		fmt.Fprintln(os.Stderr, "owned-cluster fixture configuration required")
		os.Exit(1)
	}
	if _, err := os.Stat(path); err != nil {
		fmt.Fprintln(os.Stderr, "existing fixture database required")
		os.Exit(1)
	}
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path)+"?mode=rw&_pragma=busy_timeout(2000)")
	if err != nil {
		os.Exit(1)
	}
	defer db.Close()
	conn, err := db.Conn(context.Background())
	if err != nil {
		os.Exit(1)
	}
	defer conn.Close()
	f := &fixture{token: token, path: path, conn: conn, config: gateConfig{Mode: "allow"}, gate: make(chan struct{})}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) })
	mux.HandleFunc("POST /configure", f.control(f.configure))
	mux.HandleFunc("GET /status", f.control(f.status))
	mux.HandleFunc("POST /lock", f.control(f.lock))
	mux.HandleFunc("POST /state", f.control(f.state))
	mux.HandleFunc("POST /scan", f.control(f.scan))
	server := &http.Server{Addr: ":8080", Handler: mux, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 10 * time.Second}
	tlsMux := http.NewServeMux()
	tlsMux.HandleFunc("POST /admit", f.admission)
	tlsServer := &http.Server{Addr: ":8443", Handler: tlsMux, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 30 * time.Second, TLSConfig: &tls.Config{MinVersion: tls.VersionTLS12}}
	go func() {
		if tlsServer.ListenAndServeTLS("/certs/tls.crt", "/certs/tls.key") != nil {
			fmt.Fprintln(os.Stderr, "fixture TLS server stopped")
			os.Exit(1)
		}
	}()
	if server.ListenAndServe() != nil {
		fmt.Fprintln(os.Stderr, "fixture control server stopped")
		os.Exit(1)
	}
}
