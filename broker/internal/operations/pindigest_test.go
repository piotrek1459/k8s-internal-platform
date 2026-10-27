package operations

import (
	"strings"
	"testing"
)

func TestPinDigestCanonicalisesAllReferenceForms(t *testing.T) {
	d := "sha256:" + strings.Repeat("a", 64)
	cases := []struct {
		name string
		ref  string
		want string
	}{
		{"tag only", "ghcr.io/example/app:1.0.0", "ghcr.io/example/app@" + d},
		{"digest only", "ghcr.io/example/app@" + d, "ghcr.io/example/app@" + d},
		{"tag and digest", "ghcr.io/example/app:1.0.0@" + d, "ghcr.io/example/app@" + d},
		{"registry with port", "registry.example.com:5000/team/app:tag", "registry.example.com:5000/team/app@" + d},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := pinDigest(tc.ref, d)
			if err != nil {
				t.Fatalf("pinDigest(%q): %v", tc.ref, err)
			}
			if got != tc.want {
				t.Fatalf("pinDigest(%q) = %q, want %q", tc.ref, got, tc.want)
			}
			// The canonical form must never retain a tag separator in the
			// repository portion (the bug: repo:tag landing in image.repository).
			repo := strings.SplitN(got, "@", 2)[0]
			if strings.Contains(repo, ":") && !strings.Contains(repo, ".com:") {
				// a ':' is only acceptable as a registry port (host contains a dot)
				t.Fatalf("canonical repository still contains a tag: %q", repo)
			}
		})
	}
}

func TestPinDigestRejectsInvalidReference(t *testing.T) {
	if _, err := pinDigest("not a valid ref!!", "sha256:"+strings.Repeat("a", 64)); err == nil {
		t.Fatal("expected an error for an invalid reference")
	}
}
