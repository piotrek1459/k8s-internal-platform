// Command platform is the broker's single executable: it serves the management
// API and UI assets and runs the background operation worker and status observer.
// Step 1 wires only the HTTP server and health probes; later steps add the store,
// worker, observer, and startup recovery (which runs before serving).
package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"k8s-internal-platform/broker/internal/applications"
	"k8s-internal-platform/broker/internal/auth"
	"k8s-internal-platform/broker/internal/cluster"
	"k8s-internal-platform/broker/internal/helm"
	"k8s-internal-platform/broker/internal/httpapi"
	"k8s-internal-platform/broker/internal/operations"
	"k8s-internal-platform/broker/internal/platform"
	"k8s-internal-platform/broker/internal/registry"
	"k8s-internal-platform/broker/internal/store"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, logger); err != nil {
		logger.Error("platform_failed", "error", err.Error())
		os.Exit(1)
	}
}

func run(ctx context.Context, logger *slog.Logger) error {
	addr := envOr("PLATFORM_LISTEN", "0.0.0.0:8080")
	dbPath := envOr("PLATFORM_DB", "/var/lib/platform/platform.db")

	db, err := store.Open(ctx, dbPath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close()
	st := store.New(db)
	logger.Info("store_ready", "path", dbPath)

	authSvc, err := newAuthService(ctx, st, logger)
	if err != nil {
		return err
	}

	// Cluster access is in-cluster only. Outside a cluster (local development),
	// the observer, secret storage, and readiness cluster-check are unavailable,
	// but the API still serves so the data layer can be exercised. The namespace
	// the broker manages is configurable for tests and alternate installs.
	appsNamespace := envOr("PLATFORM_APPS_NAMESPACE", "platform-apps")
	var clusterClient *cluster.Client
	if c, cerr := cluster.NewInCluster(appsNamespace); cerr != nil {
		logger.Warn("cluster_unavailable", "error", cerr.Error(),
			"note", "running without in-cluster access; observer and secret storage are disabled")
	} else {
		clusterClient = c
		logger.Info("cluster_ready", "namespace", appsNamespace)
	}

	cfg := httpapi.Config{
		Logger:        logger,
		Store:         st,
		Factory:       applications.Factory{IDs: platform.RandomIDGenerator{}, Clock: platform.SystemClock{}},
		Auth:          authSvc,
		SecureCookies: os.Getenv("PLATFORM_SECURE_COOKIES") == "true",
		Inspector:     registry.NewInspector(nil),
	}

	// The Helm adapter and the operation worker require in-cluster access. When it
	// is available, run startup recovery before serving so an interrupted mutation
	// is resumed or durably blocked before any new work is accepted, then start the
	// worker and wire its wake into the API.
	var worker *operations.Worker
	if clusterClient != nil {
		cfg.Cluster = clusterClient
		cfg.Secrets = clusterClient

		helmAdapter, herr := helm.NewAdapter(appsNamespace, cluster.HelmActionConfig(appsNamespace, logger))
		if herr != nil {
			return fmt.Errorf("init helm adapter: %w", herr)
		}
		inspector := registry.NewInspector(nil)
		worker = operations.NewWorker(st, helmAdapter, clusterClient, inspector, platform.SystemClock{}, logger)

		recovery := operations.NewRecovery(st, helmAdapter, logger)
		if rerr := recovery.Scan(ctx); rerr != nil {
			return fmt.Errorf("startup recovery: %w", rerr)
		}
		logger.Info("recovery_complete")
		cfg.Enqueuer = worker
	}

	router := httpapi.NewRouter(cfg)
	server := &http.Server{
		Handler:           router.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    64 << 10,
		ErrorLog:          slog.NewLogLogger(logger.Handler(), slog.LevelError),
	}

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	logger.Info("platform_started", "address", listener.Addr().String())

	// Start the operation worker (when cluster access is available). It stops when
	// ctx is cancelled during shutdown.
	if worker != nil {
		go worker.Run(ctx)
		logger.Info("worker_started")
	}

	result := make(chan error, 1)
	go func() { result <- server.Serve(listener) }()
	select {
	case err := <-result:
		if !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("serve HTTP: %w", err)
		}
		return nil
	case <-ctx.Done():
		logger.Info("platform_stopping")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			_ = server.Close()
			return fmt.Errorf("shutdown HTTP server: %w", err)
		}
		if err := <-result; !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("serve HTTP during shutdown: %w", err)
		}
		logger.Info("platform_stopped")
		return nil
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// bootstrapTTL bounds how long the first-run setup token remains valid.
const bootstrapTTL = 60 * time.Minute

// newAuthService builds the authentication service. When no administrator exists
// yet, it generates a single-use bootstrap token and prints it to stderr for the
// local administrator to use once during setup; the token is never persisted. The
// token can be supplied explicitly via PLATFORM_BOOTSTRAP_TOKEN (for an installer
// that shows it to the operator). Once an administrator exists, no token is
// configured and the setup endpoint is closed.
func newAuthService(ctx context.Context, st platform.Store, logger *slog.Logger) (*auth.Service, error) {
	_, adminExists, err := st.GetAdministrator(ctx)
	if err != nil {
		return nil, fmt.Errorf("check administrator: %w", err)
	}
	ids := platform.RandomIDGenerator{}
	clock := platform.SystemClock{}
	if adminExists {
		return auth.NewService(st, ids, clock, "", 0), nil
	}

	token := os.Getenv("PLATFORM_BOOTSTRAP_TOKEN")
	if token == "" {
		b := make([]byte, 24)
		if _, err := rand.Read(b); err != nil {
			return nil, fmt.Errorf("generate bootstrap token: %w", err)
		}
		token = base64.RawURLEncoding.EncodeToString(b)
		// Printed to stderr, not through the structured logger, so it is easy to
		// read from a console and is not mingled with JSON log shipping.
		fmt.Fprintf(os.Stderr, "\nBootstrap token (valid %s, single use):\n  %s\n\n", bootstrapTTL, token)
	}
	logger.Info("bootstrap_setup_available", "ttl", bootstrapTTL.String())
	return auth.NewService(st, ids, clock, token, bootstrapTTL), nil
}
