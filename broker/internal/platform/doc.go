// Package platform holds the broker's pure domain types and the port interfaces
// that adapters (store, helm, cluster, registry) implement. It depends on no
// other internal package, so every layer can import it without creating a cycle.
//
// The types here describe the data model in docs/api-and-data.md. Behaviour that
// enforces policy (validation, state transitions) lives in the applications and
// operations packages, which operate on these types through the ports below.
package platform

// OperationState is the durable lifecycle state of an operation. The JSON
// encodings match docs/api-and-data.md: NeedsAttention serialises as
// "needs_attention".
type OperationState string

const (
	OperationQueued         OperationState = "queued"
	OperationRunning        OperationState = "running"
	OperationRecovering     OperationState = "recovering"
	OperationSucceeded      OperationState = "succeeded"
	OperationFailed         OperationState = "failed"
	OperationNeedsAttention OperationState = "needs_attention"
)

// IsTerminal reports whether no further work will change the operation. Queued,
// Running, Recovering, and NeedsAttention are nonterminal; the worker and the
// startup recovery scan only consider nonterminal operations.
func (s OperationState) IsTerminal() bool {
	return s == OperationSucceeded || s == OperationFailed
}

// OperationType names the kind of mutation an operation performs. The execution
// path differs per type: only Create and Update(image) resolve a new digest;
// the others reuse a recorded digest (see docs/operations.md).
type OperationType string

const (
	OperationCreate   OperationType = "create"
	OperationUpdate   OperationType = "update"
	OperationRestart  OperationType = "restart"
	OperationReapply  OperationType = "reapply"
	OperationRollback OperationType = "rollback"
	OperationDelete   OperationType = "delete"
)

// WorkloadPhase is the observed state of the running workload, distinct from the
// operation state. A succeeded operation does not imply a ready workload.
type WorkloadPhase string

const (
	WorkloadDeploying WorkloadPhase = "deploying"
	WorkloadUpdating  WorkloadPhase = "updating"
	WorkloadReady     WorkloadPhase = "ready"
	WorkloadDegraded  WorkloadPhase = "degraded"
	WorkloadStopped   WorkloadPhase = "stopped"
	WorkloadFailed    WorkloadPhase = "failed"
	WorkloadUnknown   WorkloadPhase = "unknown"
)

// ExposureMode is the requested network visibility of an application. Only
// internal and LAN are supported in the core prototype; public arrives with M6.
type ExposureMode string

const (
	ExposureInternal ExposureMode = "internal"
	ExposureLAN      ExposureMode = "lan"
	ExposurePublic   ExposureMode = "public"
)

// WorkloadProfile selects the chart profile. The core prototype supports the
// stateless HTTP profile only.
type WorkloadProfile string

const (
	ProfileHTTP WorkloadProfile = "http"
)
