package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"k8s-internal-platform/broker/internal/platform"
)

func TestPasswordHashRoundTrip(t *testing.T) {
	hash, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	ok, err := VerifyPassword("correct horse battery staple", hash)
	if err != nil || !ok {
		t.Fatalf("verify correct password: ok=%v err=%v", ok, err)
	}
	ok, _ = VerifyPassword("wrong password", hash)
	if ok {
		t.Fatal("wrong password verified")
	}
}

func TestVerifyPasswordRejectsMalformedHash(t *testing.T) {
	if _, err := VerifyPassword("x", "not-a-real-hash"); err == nil {
		t.Fatal("expected error for malformed hash")
	}
}

// memStore is an in-memory platform.Store subset for auth tests.
type memStore struct {
	platform.Store
	admin    *platform.Administrator
	sessions map[string]platform.Session
}

func newMemStore() *memStore { return &memStore{sessions: map[string]platform.Session{}} }

func (m *memStore) CreateAdministrator(_ context.Context, a platform.Administrator) error {
	if m.admin != nil {
		return platform.Errorf(platform.CodeConflict, "administrator already exists")
	}
	m.admin = &a
	return nil
}
func (m *memStore) GetAdministrator(_ context.Context) (platform.Administrator, bool, error) {
	if m.admin == nil {
		return platform.Administrator{}, false, nil
	}
	return *m.admin, true, nil
}
func (m *memStore) CreateSession(_ context.Context, s platform.Session) error {
	m.sessions[s.ID] = s
	return nil
}
func (m *memStore) GetSession(_ context.Context, id string) (platform.Session, error) {
	s, ok := m.sessions[id]
	if !ok {
		return platform.Session{}, platform.Errorf(platform.CodeNotFound, "no session")
	}
	return s, nil
}
func (m *memStore) RevokeSession(_ context.Context, id string) error {
	if s, ok := m.sessions[id]; ok {
		now := time.Now()
		s.RevokedAt = &now
		m.sessions[id] = s
	}
	return nil
}

type testClock struct{ t time.Time }

func (c *testClock) Now() time.Time { return c.t }

func newService(clock *testClock, token string, ttl time.Duration) (*Service, *memStore) {
	store := newMemStore()
	svc := NewService(store, platform.RandomIDGenerator{}, clock, token, ttl)
	return svc, store
}

func TestSetupIsSingleUseAndExpiring(t *testing.T) {
	ctx := context.Background()
	clock := &testClock{t: time.Unix(1_000_000, 0).UTC()}
	svc, store := newService(clock, "boot-token", time.Hour)

	// Available before setup.
	if ok, _ := svc.SetupAvailable(ctx); !ok {
		t.Fatal("setup should be available initially")
	}
	// Wrong token rejected.
	if err := svc.Setup(ctx, "wrong", "a-long-enough-password"); err == nil {
		t.Fatal("wrong bootstrap token accepted")
	}
	// Correct token creates the admin.
	if err := svc.Setup(ctx, "boot-token", "a-long-enough-password"); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if store.admin == nil {
		t.Fatal("administrator not created")
	}
	// Token is consumed: a replay fails and setup is no longer available.
	if err := svc.Setup(ctx, "boot-token", "another-password"); err == nil {
		t.Fatal("bootstrap token was reusable")
	}
	if ok, _ := svc.SetupAvailable(ctx); ok {
		t.Fatal("setup should be unavailable after completion")
	}
}

func TestSetupTokenExpires(t *testing.T) {
	ctx := context.Background()
	clock := &testClock{t: time.Unix(1_000_000, 0).UTC()}
	svc, _ := newService(clock, "boot-token", time.Hour)

	clock.t = clock.t.Add(2 * time.Hour) // past expiry
	if ok, _ := svc.SetupAvailable(ctx); ok {
		t.Fatal("setup should be unavailable after token expiry")
	}
	if err := svc.Setup(ctx, "boot-token", "a-long-enough-password"); err == nil {
		t.Fatal("expired bootstrap token accepted")
	}
}

func TestSetupValidatesPasswordAndPreservesTokenOnInvalid(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name     string
		password string
		ok       bool
	}{
		{"empty", "", false},
		{"too short", "short", false},
		{"one below min", "elevenchars", false},                      // 11 chars
		{"min boundary", "twelvechars!", true},                       // 12 chars
		{"overlong", string(make([]byte, MaxPasswordLen+1)), false},  // 257 chars
		{"max boundary", string(make([]byte, MaxPasswordLen)), true}, // 256 chars
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			clock := &testClock{t: time.Unix(1_000_000, 0).UTC()}
			svc, store := newService(clock, "boot-token", time.Hour)
			err := svc.Setup(ctx, "boot-token", tc.password)
			if tc.ok {
				if err != nil {
					t.Fatalf("valid password rejected: %v", err)
				}
				if store.admin == nil {
					t.Fatal("valid setup did not create an administrator")
				}
				return
			}
			// Invalid: must be an INVALID_INPUT error, no admin created, and the
			// bootstrap token must remain usable for a subsequent valid setup.
			var derr *platform.Error
			if !errors.As(err, &derr) || derr.Code != platform.CodeInvalidInput {
				t.Fatalf("expected INVALID_INPUT, got %v", err)
			}
			if store.admin != nil {
				t.Fatal("invalid setup created an administrator")
			}
			if ok, _ := svc.SetupAvailable(ctx); !ok {
				t.Fatal("bootstrap token was consumed by an invalid setup attempt")
			}
			if err := svc.Setup(ctx, "boot-token", "a-valid-password"); err != nil {
				t.Fatalf("token not usable after invalid attempt: %v", err)
			}
		})
	}
}

func TestLoginRejectsEmptyAndOverlongInput(t *testing.T) {
	ctx := context.Background()
	clock := &testClock{t: time.Unix(1_000_000, 0).UTC()}
	svc, _ := newService(clock, "boot-token", time.Hour)
	if err := svc.Setup(ctx, "boot-token", "a-valid-password"); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if _, _, err := svc.Login(ctx, ""); err == nil {
		t.Fatal("empty login password accepted")
	}
	if _, _, err := svc.Login(ctx, string(make([]byte, MaxPasswordLen+1))); err == nil {
		t.Fatal("overlong login password accepted")
	}
}

func TestLoginSessionLifecycle(t *testing.T) {
	ctx := context.Background()
	clock := &testClock{t: time.Unix(1_000_000, 0).UTC()}
	svc, _ := newService(clock, "boot-token", time.Hour)
	if err := svc.Setup(ctx, "boot-token", "a-long-enough-password"); err != nil {
		t.Fatalf("setup: %v", err)
	}

	// Wrong password fails.
	if _, _, err := svc.Login(ctx, "nope"); err == nil {
		t.Fatal("login with wrong password succeeded")
	}
	// Correct password creates a session.
	token, _, err := svc.Login(ctx, "a-long-enough-password")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if _, _, err := svc.Authenticate(ctx, token); err != nil {
		t.Fatalf("authenticate fresh session: %v", err)
	}
	// After expiry the session is rejected.
	clock.t = clock.t.Add(SessionTTL + time.Minute)
	if _, _, err := svc.Authenticate(ctx, token); err == nil {
		t.Fatal("expired session accepted")
	}
	// Reset the clock and verify revocation.
	clock.t = time.Unix(1_000_000, 0).UTC()
	token2, _, _ := svc.Login(ctx, "a-long-enough-password")
	if err := svc.Logout(ctx, token2); err != nil {
		t.Fatalf("logout: %v", err)
	}
	if _, _, err := svc.Authenticate(ctx, token2); err == nil {
		t.Fatal("revoked session accepted")
	}
}

func TestAuthenticateRejectsEmptyAndUnknownTokens(t *testing.T) {
	ctx := context.Background()
	svc, _ := newService(&testClock{t: time.Now()}, "boot", time.Hour)
	if _, _, err := svc.Authenticate(ctx, ""); err == nil {
		t.Fatal("empty token accepted")
	}
	if _, _, err := svc.Authenticate(ctx, "unknown"); err == nil {
		t.Fatal("unknown token accepted")
	}
}
