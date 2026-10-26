package platform

import "time"

// Administrator is the single operator account. The password hash is argon2id;
// the plaintext password is never stored.
type Administrator struct {
	ID           string
	PasswordHash string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// Session is a server-side browser session. Only the hash of the session token
// is stored, so a database read cannot recover a live session credential.
type Session struct {
	ID        string // hashed session identifier
	AdminID   string
	CreatedAt time.Time
	ExpiresAt time.Time
	RevokedAt *time.Time
}

// Application is the stable record for one managed application. Version is an
// optimistic-lock counter surfaced as the ETag; ActiveOperationID is the 409
// lock that blocks concurrent mutations (including a NeedsAttention block).
type Application struct {
	ID                     string
	Name                   string
	Version                int64
	DesiredRevisionID      string
	LastSuccessfulRevision string // revision ID, empty until a first success
	ActiveOperationID      string // empty when no operation holds the lock
	Deleted                bool
	CreatedAt              time.Time
	UpdatedAt              time.Time
}

// ResourceRequirements is the request/limit preset for an application container.
// The API uses flat fields; the helm adapter maps them to the chart's nested
// resources block.
type ResourceRequirements struct {
	CPURequest    string
	CPULimit      string
	MemoryRequest string
	MemoryLimit   string
}

// SecretRef binds an environment variable to a specific immutable secret version.
type SecretRef struct {
	EnvironmentVariable string
	SecretID            string
	Version             int
}

// Exposure describes the requested visibility and, for routed modes, the hostname.
type Exposure struct {
	Mode     ExposureMode
	Hostname string
}

// Health captures the optional readiness configuration. An empty ReadinessPath
// selects the documented TCP readiness fallback.
type Health struct {
	ReadinessPath string
}

// RevisionSpec is the requested, mutable intent for a revision before it is
// frozen. The image reference may still be a tag at this point.
type RevisionSpec struct {
	Image                string
	Profile              WorkloadProfile
	ContainerPort        int
	Replicas             int
	Environment          map[string]string
	SecretRefs           []SecretRef
	Resources            ResourceRequirements
	Health               Health
	Exposure             Exposure
	RegistryCredentialID string
}

// FrozenSpec is the immutable execution specification committed before any Helm
// mutation. A retry of the same operation reuses this verbatim, so a moved tag
// cannot change what runs. See FreezeExecutionSpec on Store.
type FrozenSpec struct {
	ImageDigest         string // image@sha256:...
	ChartVersion        string
	ChartArtifactDigest string
	SecretVersions      []SecretRef
	ValuesHash          string
	FrozenAt            time.Time
}

// RevisionResult records the terminal outcome of a revision.
type RevisionResult string

const (
	RevisionPending   RevisionResult = "pending"
	RevisionSucceeded RevisionResult = "succeeded"
	RevisionFailed    RevisionResult = "failed"
)

// Revision is a candidate configuration and, once frozen, its execution spec.
type Revision struct {
	ID      string
	AppID   string
	Number  int64
	Spec    RevisionSpec
	Frozen  *FrozenSpec // nil until FreezeExecutionSpec runs
	Result  RevisionResult
	Actor   string
	Created time.Time
}

// Operation is a durable unit of work. IdempotencyKey plus RequestHash deduplicate
// retried submissions; RecoveryOutcome records what a recovery attempt concluded.
type Operation struct {
	ID              string
	AppID           string
	Type            OperationType
	SourceRevision  string
	TargetRevision  string
	State           OperationState
	Stage           string
	IdempotencyKey  string
	RequestHash     string
	RecoveryOutcome string
	CreatedAt       time.Time
	UpdatedAt       time.Time
	DeadlineAt      *time.Time
	ClaimedAt       *time.Time
}

// OperationEvent is an ordered, sanitized progress entry for an operation.
type OperationEvent struct {
	ID          int64
	OperationID string
	At          time.Time
	Stage       string
	Message     string
	RequestID   string
}

// AuditEvent records who requested what and when. Configuration diffs are
// sanitized and never contain secret values.
type AuditEvent struct {
	ID          int64
	Actor       string
	Action      string
	Target      string
	At          time.Time
	Outcome     string
	RequestID   string
	OperationID string
	Diff        string
}

// SecretMetadata describes a stored configuration secret without its value. Each
// version is an immutable Kubernetes Secret referenced by K8sSecretName.
type SecretMetadata struct {
	ID        string
	Owner     string
	Name      string
	CreatedAt time.Time
}

// SecretVersion is one immutable version of a secret. The plaintext lives only in
// the referenced Kubernetes Secret, never here.
type SecretVersion struct {
	SecretID      string
	Version       int
	Keys          []string
	K8sSecretName string
	K8sSecretUID  string
	CreatedAt     time.Time
}

// ObservedStatus is a point-in-time observation of a workload. Stale marks an
// observation the broker could not refresh because the cluster was unreachable;
// ObservedAt then refers to the last successful read.
type ObservedStatus struct {
	Phase              WorkloadPhase
	DesiredReplicas    int
	ReadyReplicas      int
	WorkloadReady      bool
	RouteReady         bool
	ObservedGeneration int64
	ObservedAt         time.Time
	Stale              bool
}
