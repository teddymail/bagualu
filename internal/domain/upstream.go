package domain

import "time"

// UpstreamFormat identifies the format of a subscription upstream.
type UpstreamFormat string

const (
	UpstreamFormatClash   UpstreamFormat = "clash"
	UpstreamFormatBase64  UpstreamFormat = "base64"
	UpstreamFormatSingBox UpstreamFormat = "singbox"
	UpstreamFormatRaw     UpstreamFormat = "raw"
)

// Upstream is a subscription source from which proxy nodes are parsed.
type Upstream struct {
	ID              string
	Name            string
	URL             string
	Format          UpstreamFormat
	RefreshInterval time.Duration // how often to re-fetch
	Enabled         bool
	Notes           string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// RefreshRecord records the outcome of one upstream refresh operation.
type RefreshRecord struct {
	ID         string
	UpstreamID string
	Success    bool
	Error      string
	NodeCount  int
	CreatedAt  time.Time
}

// NodeSource records the upstream origin of a node, preserving the original
// name and raw subscription fragment for audit and deduplication purposes.
type NodeSource struct {
	NodeID       string
	UpstreamID   string
	OriginalName string
	RawFragment  string
	CreatedAt    time.Time
}
