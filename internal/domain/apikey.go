package domain

import "time"

// APIKey is a credential that grants an external client access to a resource group.
//
// Security: the full plaintext key is NEVER stored. Only its SHA-256 hex digest
// (KeyHash) and a short display prefix (Prefix) are persisted.
type APIKey struct {
	ID        string
	Name      string
	GroupID   string
	KeyHash   string     // SHA-256 hex of the full key; used for lookup on each request
	Prefix    string     // first few characters for safe display (e.g., "bg_a1b2c3d4")
	ExpiresAt *time.Time // nil means no expiry
	RevokedAt *time.Time // non-nil once the key has been revoked
	CreatedAt time.Time
	UpdatedAt time.Time
}

// IsRevoked reports whether the key has been explicitly revoked.
func (k *APIKey) IsRevoked() bool { return k.RevokedAt != nil }

// IsExpired reports whether the key has passed its expiry time.
func (k *APIKey) IsExpired(now time.Time) bool {
	return k.ExpiresAt != nil && !now.Before(*k.ExpiresAt)
}

// IsActive reports whether the key can be used at the given time.
func (k *APIKey) IsActive(now time.Time) bool {
	return !k.IsRevoked() && !k.IsExpired(now)
}
