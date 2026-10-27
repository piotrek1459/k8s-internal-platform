package platform

import (
	"crypto/rand"
	"encoding/hex"
	"time"
)

// SystemClock is the production Clock backed by the wall clock.
type SystemClock struct{}

func (SystemClock) Now() time.Time { return time.Now().UTC() }

// RandomIDGenerator produces identifiers of the form "<prefix>_<16 hex chars>".
// The prefix mirrors the illustrative IDs in docs/api-and-data.md (app_, op_,
// rev_, sec_). Collisions are avoided by 64 bits of randomness, not a counter,
// so IDs do not leak ordering.
type RandomIDGenerator struct{}

func (RandomIDGenerator) NewID(prefix string) string {
	var b [8]byte
	// crypto/rand.Read never returns an error on supported platforms; a failure
	// here is unrecoverable, so a panic is appropriate rather than a silent weak ID.
	if _, err := rand.Read(b[:]); err != nil {
		panic("platform: cannot read random bytes for ID: " + err.Error())
	}
	return prefix + "_" + hex.EncodeToString(b[:])
}
