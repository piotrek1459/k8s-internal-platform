package applications

import (
	"strings"

	"k8s-internal-platform/broker/internal/platform"
)

// UpdateInput is the validated intent to update an existing application.
// All fields are pointers so the handler can distinguish "not provided" from
// "set to zero". The merge strategy is replace: every provided field overwrites
// the current spec entirely for that field.
type UpdateInput struct {
	Image                *string
	RegistryCredentialID *string
	ContainerPort        *int
	Replicas             *int
	Environment          map[string]string // nil = keep current; non-nil = replace
	SecretRefs           []platform.SecretRef
	Resources            *platform.ResourceRequirements
	ReadinessPath        *string
	Exposure             *platform.Exposure
}

// ValidateUpdate merges the provided update fields onto the current spec and
// validates the result. current is the spec of the application's desired revision.
func ValidateUpdate(current platform.RevisionSpec, in UpdateInput) (platform.RevisionSpec, error) {
	next := current // start from the current spec

	if in.Image != nil {
		if strings.TrimSpace(*in.Image) == "" {
			return platform.RevisionSpec{}, platform.FieldErrorf(platform.CodeInvalidInput, "image",
				"image reference must not be empty")
		}
		next.Image = *in.Image
	}
	if in.RegistryCredentialID != nil {
		next.RegistryCredentialID = *in.RegistryCredentialID
	}
	if in.ContainerPort != nil {
		if *in.ContainerPort < 1 || *in.ContainerPort > 65535 {
			return platform.RevisionSpec{}, platform.FieldErrorf(platform.CodeInvalidInput, "containerPort",
				"containerPort must be between 1 and 65535")
		}
		next.ContainerPort = *in.ContainerPort
	}
	if in.Replicas != nil {
		if *in.Replicas < 0 {
			return platform.RevisionSpec{}, platform.FieldErrorf(platform.CodeInvalidInput, "replicas",
				"replicas must not be negative")
		}
		next.Replicas = *in.Replicas
	}
	if in.Environment != nil {
		next.Environment = in.Environment
	}
	if in.SecretRefs != nil {
		next.SecretRefs = in.SecretRefs
	}
	if in.Resources != nil {
		next.Resources = *in.Resources
	}
	if in.ReadinessPath != nil {
		if *in.ReadinessPath != "" && !strings.HasPrefix(*in.ReadinessPath, "/") {
			return platform.RevisionSpec{}, platform.FieldErrorf(platform.CodeInvalidInput, "health.readinessPath",
				"readiness path must start with '/'")
		}
		next.Health.ReadinessPath = *in.ReadinessPath
	}
	if in.Exposure != nil {
		if err := validateExposure(*in.Exposure); err != nil {
			return platform.RevisionSpec{}, err
		}
		next.Exposure = *in.Exposure
	}

	// Re-validate the combined environment after merge.
	if err := validateEnvironment(next.Environment, next.SecretRefs); err != nil {
		return platform.RevisionSpec{}, err
	}

	return next, nil
}
