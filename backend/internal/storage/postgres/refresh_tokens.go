package postgres

import (
	"crypto/sha256"
	"encoding/hex"
)

// refreshTokenHash is the only representation persisted or compared in SQL.
// Refresh tokens are random bearer secrets; a stored digest must never be
// accepted directly as a token or matched through a plaintext fallback.
func refreshTokenHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
