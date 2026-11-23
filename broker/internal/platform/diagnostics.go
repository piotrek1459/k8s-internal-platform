package platform

import (
	"context"
	"time"
)

const (
	MaxLogBytes         = 64 * 1024
	MaxLogLines         = 500
	MaxDiagnosticEvents = 100
)

type LogOptions struct {
	Pod          string
	Previous     bool
	TailLines    int64
	SinceSeconds int64
}

type LogResult struct {
	Pod        string    `json:"pod"`
	Container  string    `json:"container"`
	Text       string    `json:"text"`
	Truncated  bool      `json:"truncated"`
	ObservedAt time.Time `json:"observedAt"`
}

type WorkloadEvent struct {
	At       time.Time `json:"at"`
	Type     string    `json:"type"`
	Reason   string    `json:"reason"`
	Message  string    `json:"message"`
	Resource string    `json:"resource"`
	Count    int32     `json:"count"`
}

type EventResult struct {
	Items      []WorkloadEvent `json:"items"`
	Truncated  bool            `json:"truncated"`
	ObservedAt time.Time       `json:"observedAt"`
}

// Drift differences contain field paths and classifications, never values.
type DriftDifference struct {
	Resource string `json:"resource"`
	Field    string `json:"field"`
	Change   string `json:"change"`
}

type DriftReport struct {
	RevisionID         string            `json:"revisionId"`
	ApplicationVersion int64             `json:"applicationVersion"`
	OperationActive    bool              `json:"operationActive"`
	State              string            `json:"state"`
	Differences        []DriftDifference `json:"differences"`
	ObservedAt         time.Time         `json:"observedAt"`
}

type ApplicationDiagnostics interface {
	Logs(context.Context, string, LogOptions) (LogResult, error)
	Events(context.Context, string, int) (EventResult, error)
	Drift(context.Context, string) (DriftReport, error)
}

// DiagnosticReader is namespace-bound. Implementations enforce resource
// ownership before opening a log stream or returning workload events.
type DiagnosticReader interface {
	ReadLogs(context.Context, string, LogOptions, int64) (LogResult, error)
	ReadEvents(context.Context, string, int) (EventResult, error)
	RedactionValues(context.Context) ([][]byte, error)
	CompareManifest(context.Context, string, string) ([]DriftDifference, error)
}

type ManifestRenderer interface {
	RenderExecution(string, Revision) (string, error)
}
