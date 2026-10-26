package platform

import (
	"context"
	"time"
)

// Clock abstracts time for deterministic tests.
type Clock interface {
	Now() time.Time
}

// IDGenerator produces opaque unique identifiers for applications, revisions,
// operations, and secrets.
type IDGenerator interface {
	NewID(prefix string) string
}

// AcceptResult is returned when an operation is durably accepted. The caller maps
// it to a 202 response with the operation URL. Deduplicated reports that an
// identical idempotent request returned the original operation.
type AcceptResult struct {
	ApplicationID string
	OperationID   string
	State         OperationState
	Deduplicated  bool
}

// AcceptRequest carries everything needed to persist one accepted mutation in a
// single transaction: the (possibly new) application, its candidate revision, the
// operation, and the originating actor/request for the audit trail.
type AcceptRequest struct {
	Type           OperationType
	Application    Application  // existing or newly minted; IsNew distinguishes
	IsNewApp       bool
	ExpectVersion  int64        // required for existing apps (If-Match); ignored when IsNewApp
	Revision       Revision
	IdempotencyKey string
	RequestHash    string
	Actor          string
	RequestID      string
}

// Store is the durable persistence port backed by SQLite. All mutations that must
// be atomic are expressed as single methods so the implementation can wrap them
// in one transaction; callers never compose their own cross-call transactions.
type Store interface {
	// AcceptOperation persists {application, candidate revision, operation, first
	// event, audit event} atomically. It enforces the optimistic version check
	// (CodeVersionConflict), the active-operation lock (CodeConflict), and
	// idempotency: an identical key+hash returns the original operation with
	// Deduplicated=true; the same key with a different hash returns
	// CodeIdempotencyConflict.
	AcceptOperation(ctx context.Context, req AcceptRequest) (AcceptResult, error)

	GetApplication(ctx context.Context, id string) (Application, error)
	ListApplications(ctx context.Context, limit, offset int) ([]Application, error)
	GetRevision(ctx context.Context, id string) (Revision, error)
	ListRevisions(ctx context.Context, appID string) ([]Revision, error)
	GetOperation(ctx context.Context, id string) (Operation, error)
	ListOperationEvents(ctx context.Context, operationID string) ([]OperationEvent, error)

	// ClaimNextOperation atomically marks and returns the oldest nonterminal
	// operation eligible to run, skipping applications blocked by an unresolved
	// NeedsAttention operation. It returns ok=false when nothing is claimable.
	ClaimNextOperation(ctx context.Context) (op Operation, ok bool, err error)

	// ListNonterminalOperations returns every operation that is not terminal, for
	// the startup recovery scan.
	ListNonterminalOperations(ctx context.Context) ([]Operation, error)

	// FreezeExecutionSpec records the immutable execution spec on a revision. It
	// must be called, and committed, before any external cluster mutation.
	FreezeExecutionSpec(ctx context.Context, revisionID string, spec FrozenSpec) error

	// AppendOperationEvent records a sanitized progress entry.
	AppendOperationEvent(ctx context.Context, ev OperationEvent) error

	// TransitionOperation updates an operation's state/stage and, for terminal or
	// NeedsAttention transitions, releases or retains the application lock
	// accordingly. recoveryOutcome may be empty.
	TransitionOperation(ctx context.Context, operationID string, to OperationState, stage, recoveryOutcome string) error

	// MarkRevisionResult records a revision's terminal result and, on success,
	// advances the application's last-successful pointer.
	MarkRevisionResult(ctx context.Context, revisionID string, result RevisionResult) error

	// SaveObservedStatus caches the latest workload observation for an application.
	SaveObservedStatus(ctx context.Context, appID string, status ObservedStatus) error
	GetObservedStatus(ctx context.Context, appID string) (ObservedStatus, error)

	// Secret metadata. Values live only in Kubernetes; these rows are metadata.
	CreateSecret(ctx context.Context, meta SecretMetadata, first SecretVersion) error
	AddSecretVersion(ctx context.Context, version SecretVersion) error
	GetSecret(ctx context.Context, id string) (SecretMetadata, []SecretVersion, error)
	ListSecrets(ctx context.Context) ([]SecretMetadata, error)
	// DeleteSecret removes a secret only when no active or rollback-eligible
	// revision references any of its versions; otherwise CodeSecretInUse.
	DeleteSecret(ctx context.Context, id string) error

	// Administrator and session persistence for the auth package.
	CreateAdministrator(ctx context.Context, admin Administrator) error
	GetAdministrator(ctx context.Context) (Administrator, bool, error)
	CreateSession(ctx context.Context, s Session) error
	GetSession(ctx context.Context, id string) (Session, error)
	RevokeSession(ctx context.Context, id string) error
}

// ImageReport is the result of inspecting a registry image reference.
type ImageReport struct {
	Digest        string   // sha256:...
	Architectures []string // e.g. ["arm64","amd64"]
	SuggestedPort int      // 0 when no hint is available
}

// ImageInspector resolves and validates an image before a workload mutation. It
// returns CodeImageArchUnsupported when the required host architecture is absent,
// and CodeRegistryUnavailable (retryable) when the registry cannot be reached.
type ImageInspector interface {
	Inspect(ctx context.Context, imageRef, registryCredentialID string) (ImageReport, error)
}

// ReleaseStatus is the observed outcome of a Helm release operation.
type ReleaseStatus struct {
	Revision int
	Status   string // helm status string, e.g. "deployed", "failed"
}

// HelmInstaller renders and applies the first-party application chart. The
// adapter owns the embedded chart and the in-cluster configuration; callers pass
// a frozen revision and receive the release status.
type HelmInstaller interface {
	// InstallOrUpgrade renders the application chart from the revision's frozen
	// spec and installs or upgrades the release named for appID in platform-apps,
	// within the bounded timeout. It does not auto-rollback on failure.
	InstallOrUpgrade(ctx context.Context, appID string, rev Revision) (ReleaseStatus, error)
	// Uninstall removes the release's workload and routing resources.
	Uninstall(ctx context.Context, appID string) error
	// Status reports the current release status without mutating it.
	Status(ctx context.Context, appID string) (ReleaseStatus, bool, error)
}

// ClusterObserver reads live workload state from the Kubernetes API.
type ClusterObserver interface {
	// Observe returns the current status of the application's workload. A cluster
	// read failure is reported as a domain error so the worker/observer can mark
	// cached status stale rather than overwrite it as fresh.
	Observe(ctx context.Context, appID string) (ObservedStatus, error)
}

// SecretManager creates immutable Kubernetes Secrets for configuration values and
// registry credentials. The plaintext is written only to Kubernetes.
type SecretManager interface {
	// CreateImmutableSecret creates an immutable Secret with a deterministic
	// versioned name and returns its name and UID for metadata storage.
	CreateImmutableSecret(ctx context.Context, secretID string, version int, data map[string][]byte) (name, uid string, err error)
}
