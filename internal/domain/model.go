package domain

import "time"

type NodeStatus string

const (
	NodeActive              NodeStatus = "active"
	NodeUnreachable         NodeStatus = "unreachable"
	NodeEndpointUnreachable NodeStatus = "endpoint_ip_unreachable"
	NodeDisabled            NodeStatus = "disabled"
	NodeExpired             NodeStatus = "expired"
	NodeInvalid             NodeStatus = "invalid"
)

type Recommendation string

const (
	RecommendationUnrated                Recommendation = "unrated"
	RecommendationNotRecommended         Recommendation = "not_recommended"
	RecommendationRecommended            Recommendation = "recommended"
	RecommendationTemporarilyUnavailable Recommendation = "temporarily_unavailable"
)

type Node struct {
	ID, Name, Protocol, Address, EndpointIP, ExitIP, Country, City, ASN, Organization, Region, SourceURL, GeoSource string
	GeoUpdatedAt, RegionChangedAt                                                                                   *time.Time
	Port                                                                                                            int
	Status                                                                                                          NodeStatus
	Score                                                                                                           *Score
	LastLatencyMS                                                                                                   *float64
	LastSpeedBPS                                                                                                    *float64
	RawConfig                                                                                                       map[string]any
	CreatedAt                                                                                                       time.Time
	UpdatedAt                                                                                                       time.Time
}

type Score struct {
	Latency, Speed, Availability, Overall             float64
	Status                                            Recommendation
	LatencySamples, SpeedSamples, AvailabilitySamples int
	StrategyVersion                                   int
	CalculatedAt                                      time.Time
}

type Measurement struct {
	ID, NodeID, Kind, ErrorCode, FailureStage                               string
	Success                                                                 bool
	LatencyMS, FirstByteMS, SpeedBytesPerSec, EffectiveDownloadDurationMS   float64
	Bytes                                                                   int64
	UploadBytes                                                             int64
	WANDownloadBefore, WANDownloadAfter, WANUploadBefore, WANUploadAfter    int64
	ProxyProtocol, TestURL, ExitIP, BaselineTarget, SpeedSource, LoadStatus string
	BackgroundUploadBPS, BackgroundDownloadBPS                              float64
	WANDownloadCapacityBPS, WANUploadCapacityBPS, LoadThreshold             float64
	LoadSampleDurationMS                                                    int64
	CreatedAt                                                               time.Time
	CoreEvidence                                                            CoreEvidence
	Infrastructure                                                          bool
}

type CoreEvidence struct {
	PID                string `json:"pid,omitempty"`
	InstanceID         string `json:"instance_id,omitempty"`
	Version            string `json:"version,omitempty"`
	NodeName           string `json:"node_name,omitempty"`
	ConnectionID       string `json:"connection_id,omitempty"`
	ExitIP             string `json:"exit_ip,omitempty"`
	TrafficBefore      int64  `json:"traffic_before,omitempty"`
	TrafficAfter       int64  `json:"traffic_after,omitempty"`
	UploadBefore       int64  `json:"upload_before,omitempty"`
	UploadAfter        int64  `json:"upload_after,omitempty"`
	LogicalConnections int    `json:"logical_connections,omitempty"`
	ConfigDigest       string `json:"config_digest,omitempty"`
}

type CoreStatus struct {
	Available    bool   `json:"available"`
	PID          int    `json:"pid"`
	Version      string `json:"version"`
	Control      string `json:"control"`
	Proxy        string `json:"proxy"`
	ErrorCode    string `json:"error_code,omitempty"`
	State        string `json:"state,omitempty"`
	AutoRestarts int    `json:"auto_restarts"`
}

type CoreInstallStatus struct {
	Installed    bool   `json:"installed"`
	Path         string `json:"path"`
	Version      string `json:"version,omitempty"`
	Architecture string `json:"architecture"`
	Source       string `json:"source,omitempty"`
	Error        string `json:"error,omitempty"`
}

type CoreInstallResult struct {
	Version  string `json:"version"`
	Asset    string `json:"asset"`
	Path     string `json:"path"`
	Verified bool   `json:"verified"`
	SHA256   string `json:"sha256,omitempty"`
}
