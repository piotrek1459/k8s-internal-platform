package applications

import (
	"testing"
	"time"

	"k8s-internal-platform/broker/internal/platform"
)

func validCreate() CreateInput {
	return CreateInput{
		Name:          "notes",
		Image:         "ghcr.io/example/notes:1.0.0",
		ContainerPort: 8080,
		Profile:       "http",
		Exposure:      platform.Exposure{Mode: platform.ExposureInternal},
	}
}

func TestValidateCreateAcceptsMinimalValidInput(t *testing.T) {
	spec, err := ValidateCreate(validCreate())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if spec.Replicas != 1 {
		t.Fatalf("default replicas = %d, want 1", spec.Replicas)
	}
	if spec.Profile != platform.ProfileHTTP {
		t.Fatalf("profile = %q", spec.Profile)
	}
}

func TestValidateCreateRejectsInvalidInput(t *testing.T) {
	zero := 0
	neg := -1
	tests := []struct {
		name      string
		mutate    func(*CreateInput)
		wantCode  platform.ErrorCode
		wantField string
	}{
		{"bad name uppercase", func(c *CreateInput) { c.Name = "Notes" }, platform.CodeInvalidInput, "name"},
		{"bad name symbol", func(c *CreateInput) { c.Name = "my_app" }, platform.CodeInvalidInput, "name"},
		{"empty image", func(c *CreateInput) { c.Image = "  " }, platform.CodeInvalidInput, "image"},
		{"port zero", func(c *CreateInput) { c.ContainerPort = 0 }, platform.CodeInvalidInput, "containerPort"},
		{"port too high", func(c *CreateInput) { c.ContainerPort = 70000 }, platform.CodeInvalidInput, "containerPort"},
		{"unsupported profile", func(c *CreateInput) { c.Profile = "tcp" }, platform.CodeUnsupportedSpec, "profile"},
		{"negative replicas", func(c *CreateInput) { c.Replicas = &neg }, platform.CodeInvalidInput, "replicas"},
		{"public exposure", func(c *CreateInput) { c.Exposure = platform.Exposure{Mode: platform.ExposurePublic} }, platform.CodeUnsupportedSpec, "exposure.mode"},
		{"lan without hostname", func(c *CreateInput) { c.Exposure = platform.Exposure{Mode: platform.ExposureLAN} }, platform.CodeInvalidInput, "exposure.hostname"},
		{"bad readiness path", func(c *CreateInput) { c.ReadinessPath = "readyz" }, platform.CodeInvalidInput, "health.readinessPath"},
		{"zero replicas allowed", func(c *CreateInput) { c.Replicas = &zero }, "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := validCreate()
			tt.mutate(&in)
			_, err := ValidateCreate(in)
			if tt.wantCode == "" {
				if err != nil {
					t.Fatalf("expected no error, got %v", err)
				}
				return
			}
			derr, ok := err.(*platform.Error)
			if !ok {
				t.Fatalf("error %v is not *platform.Error", err)
			}
			if derr.Code != tt.wantCode || derr.Field != tt.wantField {
				t.Fatalf("got code=%s field=%s, want code=%s field=%s", derr.Code, derr.Field, tt.wantCode, tt.wantField)
			}
		})
	}
}

func TestValidateCreateRejectsDuplicateEnvAndSecretVar(t *testing.T) {
	in := validCreate()
	in.Environment = map[string]string{"APP_SECRET": "plain"}
	in.SecretRefs = []platform.SecretRef{{EnvironmentVariable: "APP_SECRET", SecretID: "sec_1", Version: 1}}
	_, err := ValidateCreate(in)
	derr, ok := err.(*platform.Error)
	if !ok || derr.Code != platform.CodeInvalidInput || derr.Field != "secretRefs" {
		t.Fatalf("expected a secretRefs conflict, got %v", err)
	}
}

func TestValidateCreateAcceptsLANWithHostname(t *testing.T) {
	in := validCreate()
	in.Exposure = platform.Exposure{Mode: platform.ExposureLAN, Hostname: "notes.pi.home.arpa"}
	if _, err := ValidateCreate(in); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// fixedIDs and fixedClock make NewCreateRequest deterministic.
type fixedIDs struct{ seq int }

func (f *fixedIDs) NewID(prefix string) string {
	f.seq++
	return prefix + "_fixed"
}

type fixedClock struct{ t time.Time }

func (c fixedClock) Now() time.Time { return c.t }

func TestNewCreateRequestBuildsConsistentRequest(t *testing.T) {
	spec, err := ValidateCreate(validCreate())
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	f := Factory{IDs: &fixedIDs{}, Clock: fixedClock{t: time.Unix(0, 0).UTC()}}
	req := f.NewCreateRequest("notes", spec, "idem-1", "hash-1", "admin", "req-1")

	if !req.IsNewApp || req.Type != platform.OperationCreate {
		t.Fatalf("not a create request: %+v", req)
	}
	if req.Application.Name != "notes" {
		t.Fatalf("name = %q", req.Application.Name)
	}
	// The operation, revision, and application must share the same app ID, and the
	// operation must target the candidate revision.
	if req.Op.AppID != req.Application.ID || req.Revision.AppID != req.Application.ID {
		t.Fatalf("app IDs inconsistent: %+v", req)
	}
	if req.Op.TargetRevision != req.Revision.ID {
		t.Fatalf("operation does not target the candidate revision")
	}
	if req.Revision.Number != 1 || req.Revision.Result != platform.RevisionPending {
		t.Fatalf("unexpected revision: %+v", req.Revision)
	}
}
