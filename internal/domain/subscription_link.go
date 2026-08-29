package domain

import "time"

// SubscriptionLink is a stable public endpoint for proxy clients to fetch node lists.
// It is bound to exactly one resource group and carries default output policy.
//
// Security: the full plaintext token is NEVER stored. Only its SHA-256 hex digest
// (TokenHash) and a short display prefix (TokenPrefix) are persisted.
type SubscriptionLink struct {
	ID             string
	Name           string
	GroupID        string
	TokenHash      string   // SHA-256 hex of the full token
	TokenPrefix    string   // first few characters for safe display
	DefaultFormat  string   // clash | base64 | singbox | dae | original
	AllowedFormats []string // empty means all formats supported by the server
	MinScore       float64  // resource output threshold; application layer enforces >= 60
	Limit          int      // maximum nodes to return; 0 = unlimited
	HealthyOnly    bool     // only return nodes that pass health policy
	Enabled        bool
	ExpiresAt      *time.Time // nil means no expiry
	LastAccessAt   *time.Time // nil until first use
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// IsExpired reports whether the link has passed its expiry time.
func (l *SubscriptionLink) IsExpired(now time.Time) bool {
	return l.ExpiresAt != nil && !now.Before(*l.ExpiresAt)
}

// IsAccessible reports whether the link can currently be used by a client.
func (l *SubscriptionLink) IsAccessible(now time.Time) bool {
	return l.Enabled && !l.IsExpired(now)
}
