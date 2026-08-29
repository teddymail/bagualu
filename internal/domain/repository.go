package domain

import (
	"context"
	"time"
)

// NodeFilter parameterises FindAll queries on nodes.
// Zero values are treated as "no filter".
type NodeFilter struct {
	Status   NodeStatus // empty = all statuses
	Protocol string     // empty = all protocols
	Region   string     // empty = all regions
	GroupID  string     // empty = all groups
	Search   string     // matches name, address, endpoint IP, or source URL
	Sort     string     // approved stable sort key
	Offset   int
	Limit    int // 0 = no limit
}

// JobFilter parameterises FindAll queries on jobs.
// Zero values are treated as "no filter".
type JobFilter struct {
	Status JobStatus // empty = all statuses
	Kind   string    // empty = all kinds
	Limit  int       // 0 = no limit
}

// NodeRepository handles persistence of Node entities and their upstream source
// relationships. It must not apply business rules about minimum score or ordering.
type NodeRepository interface {
	Save(ctx context.Context, node *Node) error
	FindByID(ctx context.Context, id string) (*Node, error)
	FindAll(ctx context.Context, f NodeFilter) ([]Node, error)
	Delete(ctx context.Context, id string) error
	UpdateStatus(ctx context.Context, id string, status NodeStatus) error
	SaveNodeSource(ctx context.Context, src NodeSource) error
	FindNodeSources(ctx context.Context, nodeID string) ([]NodeSource, error)
}

// MeasurementRepository handles persistence of test Measurement results.
type MeasurementRepository interface {
	Save(ctx context.Context, m *Measurement) error
	FindByNodeID(ctx context.Context, nodeID string, limit int) ([]Measurement, error)
	FindSince(ctx context.Context, nodeID string, since time.Time) ([]Measurement, error)
	FindLatestTimesByKind(ctx context.Context, kind string, excludeInfrastructure bool) (map[string]time.Time, error)
	DeleteByNodeID(ctx context.Context, nodeID string) error
}

// UpstreamRepository handles persistence of Upstream subscriptions and their
// associated RefreshRecord history.
type UpstreamRepository interface {
	Save(ctx context.Context, u *Upstream) error
	FindByID(ctx context.Context, id string) (*Upstream, error)
	FindAll(ctx context.Context) ([]Upstream, error)
	Delete(ctx context.Context, id string) error
	SaveRefreshRecord(ctx context.Context, r *RefreshRecord) error
	FindRefreshRecords(ctx context.Context, upstreamID string, limit int) ([]RefreshRecord, error)
}

// GroupRepository handles persistence of Groups and their node memberships.
type GroupRepository interface {
	Save(ctx context.Context, g *Group) error
	FindByID(ctx context.Context, id string) (*Group, error)
	FindAll(ctx context.Context) ([]Group, error)
	Delete(ctx context.Context, id string) error
	// SetNodes replaces the complete node membership of a group atomically.
	SetNodes(ctx context.Context, groupID string, nodeIDs []string) error
	FindNodeIDs(ctx context.Context, groupID string) ([]string, error)
}

// JobRepository handles persistence of background Job records.
type JobRepository interface {
	Save(ctx context.Context, j *Job) error
	FindByID(ctx context.Context, id string) (*Job, error)
	FindAll(ctx context.Context, f JobFilter) ([]Job, error)
	DeleteInactive(ctx context.Context) error
	DeleteAll(ctx context.Context) error
	// UpdateStatus advances a job's status, progress percentage, and error message.
	UpdateStatus(ctx context.Context, id string, status JobStatus, progress int, errMsg string) error
}

// APIKeyRepository handles persistence of API key records.
// Plaintext keys are NEVER stored; only the SHA-256 hash and display prefix are persisted.
type APIKeyRepository interface {
	Save(ctx context.Context, k *APIKey) error
	FindByID(ctx context.Context, id string) (*APIKey, error)
	FindAll(ctx context.Context) ([]APIKey, error)
	// FindByKeyHash looks up a key by its SHA-256 hex digest for request authentication.
	FindByKeyHash(ctx context.Context, hash string) (*APIKey, error)
	Revoke(ctx context.Context, id string, at time.Time) error
	Delete(ctx context.Context, id string) error
}

// SubscriptionLinkRepository handles persistence of subscription link records.
// Plaintext tokens are NEVER stored; only the SHA-256 hash and display prefix are persisted.
type SubscriptionLinkRepository interface {
	Save(ctx context.Context, l *SubscriptionLink) error
	FindByID(ctx context.Context, id string) (*SubscriptionLink, error)
	FindAll(ctx context.Context) ([]SubscriptionLink, error)
	// FindByTokenHash looks up a link by its SHA-256 hex digest for subscriber access.
	FindByTokenHash(ctx context.Context, hash string) (*SubscriptionLink, error)
	// UpdateLastAccess records the time the link was last successfully accessed.
	UpdateLastAccess(ctx context.Context, id string, at time.Time) error
	Delete(ctx context.Context, id string) error
}

// SystemSettingsRepository handles persistence of system-wide key-value configuration.
// Sensitive values (e.g., admin password hash) must be hashed by the caller before storage.
type SystemSettingsRepository interface {
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key, value string) error
	GetAll(ctx context.Context) (map[string]string, error)
	Delete(ctx context.Context, key string) error
}

// ScoreSnapshotRepository handles persistence of immutable ScoreSnapshot records.
type ScoreSnapshotRepository interface {
	Save(ctx context.Context, s *ScoreSnapshot) error
	FindByNodeID(ctx context.Context, nodeID string, limit int) ([]ScoreSnapshot, error)
	FindLatestByNodeID(ctx context.Context, nodeID string) (*ScoreSnapshot, error)
}
