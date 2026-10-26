package auth

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"sync"
	"time"

	"k8s-internal-platform/broker/internal/platform"
)

// SessionTTL is how long a login session remains valid.
const SessionTTL = 12 * time.Hour

// Service performs authentication against the store. The bootstrap token lives in
// memory only: it is shown once to the local administrator at install time and is
// never persisted, so a database compromise cannot reveal it. Setup consumes it
// exactly once.
type Service struct {
	store platform.Store
	ids   platform.IDGenerator
	clock platform.Clock

	mu             sync.Mutex
	bootstrapHash  string    // sha256 of the active bootstrap token, empty once consumed
	bootstrapUntil time.Time // token expiry
}

// NewService builds an auth service. bootstrapToken is the single-use setup token;
// pass "" to disable setup (for example after setup has completed in a prior run).
func NewService(store platform.Store, ids platform.IDGenerator, clock platform.Clock, bootstrapToken string, bootstrapTTL time.Duration) *Service {
	s := &Service{store: store, ids: ids, clock: clock}
	if bootstrapToken != "" {
		s.bootstrapHash = hashToken(bootstrapToken)
		s.bootstrapUntil = clock.Now().Add(bootstrapTTL)
	}
	return s
}

// SetupAvailable reports whether initial administrator setup can still run: no
// administrator exists yet and a valid, unexpired bootstrap token is configured.
func (s *Service) SetupAvailable(ctx context.Context) (bool, error) {
	_, exists, err := s.store.GetAdministrator(ctx)
	if err != nil {
		return false, err
	}
	if exists {
		return false, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.bootstrapHash != "" && s.clock.Now().Before(s.bootstrapUntil), nil
}

// Setup consumes the bootstrap token and creates the administrator. It is
// single-use: the token is cleared on success and on a valid attempt, so a
// replayed token after expiry or after setup cannot create a second account.
func (s *Service) Setup(ctx context.Context, bootstrapToken, password string) error {
	s.mu.Lock()
	validToken := s.bootstrapHash != "" &&
		s.clock.Now().Before(s.bootstrapUntil) &&
		subtle.ConstantTimeCompare([]byte(hashToken(bootstrapToken)), []byte(s.bootstrapHash)) == 1
	s.mu.Unlock()

	if !validToken {
		return platform.Errorf(platform.CodeUnauthenticated, "invalid or expired bootstrap token")
	}

	hash, err := HashPassword(password)
	if err != nil {
		return err
	}
	now := s.clock.Now()
	admin := platform.Administrator{ID: s.ids.NewID("admin"), PasswordHash: hash, CreatedAt: now, UpdatedAt: now}
	if err := s.store.CreateAdministrator(ctx, admin); err != nil {
		return err
	}
	// Consume the token only after the administrator is durably created, so a
	// transient store error leaves setup retryable.
	s.mu.Lock()
	s.bootstrapHash = ""
	s.mu.Unlock()
	return nil
}

// Login verifies the password and creates a session, returning the raw session
// token (to be set as a cookie) and the session's expiry. The password is
// verified in constant time via argon2id; a missing administrator still performs
// a comparison-shaped amount of work by returning the same error.
func (s *Service) Login(ctx context.Context, password string) (token string, expires time.Time, err error) {
	admin, exists, err := s.store.GetAdministrator(ctx)
	if err != nil {
		return "", time.Time{}, err
	}
	if !exists {
		return "", time.Time{}, platform.Errorf(platform.CodeUnauthenticated, "authentication failed")
	}
	ok, err := VerifyPassword(password, admin.PasswordHash)
	if err != nil || !ok {
		return "", time.Time{}, platform.Errorf(platform.CodeUnauthenticated, "authentication failed")
	}

	raw, err := newToken(32)
	if err != nil {
		return "", time.Time{}, err
	}
	now := s.clock.Now()
	expires = now.Add(SessionTTL)
	sess := platform.Session{ID: hashToken(raw), AdminID: admin.ID, CreatedAt: now, ExpiresAt: expires}
	if err := s.store.CreateSession(ctx, sess); err != nil {
		return "", time.Time{}, err
	}
	return raw, expires, nil
}

// Authenticate validates a raw session token and returns the administrator ID and
// session expiry. Expired or revoked sessions are rejected.
func (s *Service) Authenticate(ctx context.Context, rawToken string) (adminID string, expires time.Time, err error) {
	if rawToken == "" {
		return "", time.Time{}, platform.Errorf(platform.CodeUnauthenticated, "no session")
	}
	sess, err := s.store.GetSession(ctx, hashToken(rawToken))
	if err != nil {
		// A missing session is an authentication failure, not a 404.
		return "", time.Time{}, platform.Errorf(platform.CodeUnauthenticated, "invalid session")
	}
	if sess.RevokedAt != nil {
		return "", time.Time{}, platform.Errorf(platform.CodeUnauthenticated, "session revoked")
	}
	if !s.clock.Now().Before(sess.ExpiresAt) {
		return "", time.Time{}, platform.Errorf(platform.CodeUnauthenticated, "session expired")
	}
	return sess.AdminID, sess.ExpiresAt, nil
}

// Logout revokes the session identified by the raw token.
func (s *Service) Logout(ctx context.Context, rawToken string) error {
	if rawToken == "" {
		return nil
	}
	return s.store.RevokeSession(ctx, hashToken(rawToken))
}

// hashToken returns the hex sha256 of a token. Only hashes are stored or compared
// so a store read cannot recover a live token or session credential.
func hashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}
