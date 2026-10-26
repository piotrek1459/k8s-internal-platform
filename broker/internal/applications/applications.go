// Package applications holds the pure lifecycle policy for managed applications:
// input validation, revision construction, and the rules that turn an API request
// into a platform.AcceptRequest for the store. It performs no I/O and depends only
// on the platform domain types, so its policy is exhaustively unit-testable.
package applications

import (
	"regexp"
	"strings"

	"k8s-internal-platform/broker/internal/platform"
)

// dns1123Label matches a Kubernetes DNS-1123 label, the form required for an
// application name used as a Helm release name and in resource names.
var dns1123Label = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)

// envName matches a valid environment variable name.
var envName = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

// CreateInput is the validated intent to create an application. It mirrors the
// POST /api/v1/apps body after JSON decoding; validation converts it into a
// revision spec.
type CreateInput struct {
	Name                 string
	Image                string
	RegistryCredentialID string
	ContainerPort        int
	Profile              string
	Replicas             *int // nil means default (1)
	Environment          map[string]string
	SecretRefs           []platform.SecretRef
	Resources            platform.ResourceRequirements
	ReadinessPath        string
	Exposure             platform.Exposure
}

// Factory builds AcceptRequests. It owns the ID generator and clock so the rest of
// the package stays free of ambient state and is deterministic under test.
type Factory struct {
	IDs   platform.IDGenerator
	Clock platform.Clock
}

// ValidateCreate checks a create request and returns the candidate revision spec.
// It returns a platform.Error with CodeUnsupportedSpec or CodeInvalidInput and the
// offending field so the HTTP layer can map it to 422/400 with a field name.
func ValidateCreate(in CreateInput) (platform.RevisionSpec, error) {
	name := strings.TrimSpace(in.Name)
	if !dns1123Label.MatchString(name) || len(name) > 63 {
		return platform.RevisionSpec{}, platform.FieldErrorf(platform.CodeInvalidInput, "name",
			"name must be a DNS-1123 label (lowercase alphanumeric and -), up to 63 characters")
	}
	if strings.TrimSpace(in.Image) == "" {
		return platform.RevisionSpec{}, platform.FieldErrorf(platform.CodeInvalidInput, "image",
			"image reference is required")
	}
	if in.ContainerPort < 1 || in.ContainerPort > 65535 {
		return platform.RevisionSpec{}, platform.FieldErrorf(platform.CodeInvalidInput, "containerPort",
			"containerPort must be between 1 and 65535")
	}
	if in.Profile != string(platform.ProfileHTTP) {
		return platform.RevisionSpec{}, platform.FieldErrorf(platform.CodeUnsupportedSpec, "profile",
			"only the %q profile is supported in the core prototype", platform.ProfileHTTP)
	}

	replicas := 1
	if in.Replicas != nil {
		replicas = *in.Replicas
	}
	if replicas < 0 {
		return platform.RevisionSpec{}, platform.FieldErrorf(platform.CodeInvalidInput, "replicas",
			"replicas must not be negative")
	}

	if err := validateEnvironment(in.Environment, in.SecretRefs); err != nil {
		return platform.RevisionSpec{}, err
	}
	if err := validateExposure(in.Exposure); err != nil {
		return platform.RevisionSpec{}, err
	}
	if in.ReadinessPath != "" && !strings.HasPrefix(in.ReadinessPath, "/") {
		return platform.RevisionSpec{}, platform.FieldErrorf(platform.CodeInvalidInput, "health.readinessPath",
			"readiness path must start with '/'")
	}

	return platform.RevisionSpec{
		Image:                in.Image,
		Profile:              platform.ProfileHTTP,
		ContainerPort:        in.ContainerPort,
		Replicas:             replicas,
		Environment:          in.Environment,
		SecretRefs:           in.SecretRefs,
		Resources:            in.Resources,
		Health:               platform.Health{ReadinessPath: in.ReadinessPath},
		Exposure:             in.Exposure,
		RegistryCredentialID: in.RegistryCredentialID,
	}, nil
}

func validateEnvironment(env map[string]string, refs []platform.SecretRef) error {
	seen := make(map[string]bool, len(env)+len(refs))
	for k := range env {
		if !envName.MatchString(k) {
			return platform.FieldErrorf(platform.CodeInvalidInput, "environment",
				"invalid environment variable name %q", k)
		}
		seen[k] = true
	}
	for _, ref := range refs {
		if !envName.MatchString(ref.EnvironmentVariable) {
			return platform.FieldErrorf(platform.CodeInvalidInput, "secretRefs",
				"invalid environment variable name %q", ref.EnvironmentVariable)
		}
		if ref.SecretID == "" || ref.Version < 1 {
			return platform.FieldErrorf(platform.CodeInvalidInput, "secretRefs",
				"each secret reference requires a secretId and a version >= 1")
		}
		// A variable set by both a plain value and a secret would be ambiguous; the
		// chart also rejects this, so fail early with a clear message.
		if seen[ref.EnvironmentVariable] {
			return platform.FieldErrorf(platform.CodeInvalidInput, "secretRefs",
				"environment variable %q is set by both environment and secretRefs", ref.EnvironmentVariable)
		}
		seen[ref.EnvironmentVariable] = true
	}
	return nil
}

func validateExposure(e platform.Exposure) error {
	switch e.Mode {
	case platform.ExposureInternal:
		return nil
	case platform.ExposureLAN:
		if strings.TrimSpace(e.Hostname) == "" {
			return platform.FieldErrorf(platform.CodeInvalidInput, "exposure.hostname",
				"a hostname is required for LAN exposure")
		}
		return nil
	case platform.ExposurePublic:
		// Public exposure requires the networking configured in M6.
		return platform.FieldErrorf(platform.CodeUnsupportedSpec, "exposure.mode",
			"public exposure is not available in the core prototype")
	default:
		return platform.FieldErrorf(platform.CodeInvalidInput, "exposure.mode",
			"unknown exposure mode %q", e.Mode)
	}
}

// NewCreateRequest builds the AcceptRequest for creating an application. name
// must already be validated (ValidateCreate does this). The caller passes the
// request hash (over the canonical body) and audit context.
func (f Factory) NewCreateRequest(name string, spec platform.RevisionSpec, idempotencyKey, requestHash, actor, requestID string) platform.AcceptRequest {
	appID := f.IDs.NewID("app")
	revID := f.IDs.NewID("rev")
	opID := f.IDs.NewID("op")
	now := f.Clock.Now()

	rev := platform.Revision{
		ID:      revID,
		AppID:   appID,
		Number:  1,
		Spec:    spec,
		Result:  platform.RevisionPending,
		Actor:   actor,
		Created: now,
	}
	op := platform.Operation{
		ID:             opID,
		AppID:          appID,
		Type:           platform.OperationCreate,
		TargetRevision: revID,
		State:          platform.OperationQueued,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	return platform.AcceptRequest{
		Type:           platform.OperationCreate,
		Application:    platform.Application{ID: appID, Name: name},
		IsNewApp:       true,
		Revision:       rev,
		Op:             op,
		IdempotencyKey: idempotencyKey,
		RequestHash:    requestHash,
		Actor:          actor,
		RequestID:      requestID,
	}
}

// ReleaseName converts an application name into a Helm release / resource name.
// Names are already validated as DNS-1123 labels, so this currently returns the
// name unchanged; it exists as the single place to adjust if naming rules change.
func ReleaseName(appName string) string { return appName }
