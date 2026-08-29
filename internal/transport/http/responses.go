package httptransport

import (
	"time"

	"github.com/teddymail/bagualu/internal/domain"
)

// Response DTOs with snake_case JSON tags, so the API follows REST conventions
// without requiring modifications to the domain package.

type upstreamResponse struct {
	ID              string                 `json:"id"`
	Name            string                 `json:"name"`
	URL             string                 `json:"url"`
	Format          domain.UpstreamFormat  `json:"format"`
	RefreshInterval int64                  `json:"refresh_interval_seconds"`
	Enabled         bool                   `json:"enabled"`
	Notes           string                 `json:"notes"`
	CreatedAt       time.Time              `json:"created_at"`
	UpdatedAt       time.Time              `json:"updated_at"`
	LastRefresh     *refreshRecordResponse `json:"last_refresh,omitempty"`
}

func toUpstreamResponse(u *domain.Upstream) upstreamResponse {
	return upstreamResponse{
		ID: u.ID, Name: u.Name, URL: u.URL, Format: u.Format,
		RefreshInterval: int64(u.RefreshInterval.Seconds()),
		Enabled:         u.Enabled, Notes: u.Notes,
		CreatedAt: u.CreatedAt, UpdatedAt: u.UpdatedAt,
	}
}

type refreshRecordResponse struct {
	ID         string    `json:"id"`
	UpstreamID string    `json:"upstream_id"`
	Success    bool      `json:"success"`
	Error      string    `json:"error,omitempty"`
	NodeCount  int       `json:"node_count"`
	CreatedAt  time.Time `json:"created_at"`
}

func toRefreshRecordResponse(r domain.RefreshRecord) refreshRecordResponse {
	return refreshRecordResponse{
		ID: r.ID, UpstreamID: r.UpstreamID, Success: r.Success,
		Error: r.Error, NodeCount: r.NodeCount, CreatedAt: r.CreatedAt,
	}
}

type groupResponse struct {
	ID               string    `json:"id"`
	Name             string    `json:"name"`
	Description      string    `json:"description"`
	MinScore         float64   `json:"min_score"`
	OnePerEndpointIP bool      `json:"one_per_endpoint_ip"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

func toGroupResponse(g *domain.Group) groupResponse {
	return groupResponse{
		ID: g.ID, Name: g.Name, Description: g.Description,
		MinScore: g.MinScore, OnePerEndpointIP: g.OnePerEndpointIP,
		CreatedAt: g.CreatedAt, UpdatedAt: g.UpdatedAt,
	}
}

type jobResponse struct {
	ID         string           `json:"id"`
	Kind       string           `json:"kind"`
	Status     domain.JobStatus `json:"status"`
	Progress   int              `json:"progress"`
	EntityID   string           `json:"entity_id,omitempty"`
	Error      string           `json:"error,omitempty"`
	CreatedAt  time.Time        `json:"created_at"`
	UpdatedAt  time.Time        `json:"updated_at"`
	FinishedAt *time.Time       `json:"finished_at,omitempty"`
}

func toJobResponse(j *domain.Job) jobResponse {
	return jobResponse{
		ID: j.ID, Kind: j.Kind, Status: j.Status, Progress: j.Progress,
		EntityID: j.EntityID, Error: j.Error,
		CreatedAt: j.CreatedAt, UpdatedAt: j.UpdatedAt, FinishedAt: j.FinishedAt,
	}
}

type apiKeyResponse struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	GroupID   string     `json:"group_id"`
	Prefix    string     `json:"prefix"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	RevokedAt *time.Time `json:"revoked_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}

func toAPIKeyResponse(k *domain.APIKey) apiKeyResponse {
	return apiKeyResponse{
		ID: k.ID, Name: k.Name, GroupID: k.GroupID, Prefix: k.Prefix,
		ExpiresAt: k.ExpiresAt, RevokedAt: k.RevokedAt,
		CreatedAt: k.CreatedAt, UpdatedAt: k.UpdatedAt,
	}
}

type subscriptionLinkResponse struct {
	ID             string     `json:"id"`
	Name           string     `json:"name"`
	GroupID        string     `json:"group_id"`
	TokenPrefix    string     `json:"token_prefix"`
	DefaultFormat  string     `json:"default_format"`
	AllowedFormats []string   `json:"allowed_formats,omitempty"`
	MinScore       float64    `json:"min_score"`
	Limit          int        `json:"limit"`
	HealthyOnly    bool       `json:"healthy_only"`
	Enabled        bool       `json:"enabled"`
	ExpiresAt      *time.Time `json:"expires_at,omitempty"`
	LastAccessAt   *time.Time `json:"last_access_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

func toSubscriptionLinkResponse(l *domain.SubscriptionLink) subscriptionLinkResponse {
	return subscriptionLinkResponse{
		ID: l.ID, Name: l.Name, GroupID: l.GroupID, TokenPrefix: l.TokenPrefix,
		DefaultFormat: l.DefaultFormat, MinScore: l.MinScore, Limit: l.Limit,
		AllowedFormats: l.AllowedFormats,
		HealthyOnly:    l.HealthyOnly, Enabled: l.Enabled,
		ExpiresAt: l.ExpiresAt, LastAccessAt: l.LastAccessAt,
		CreatedAt: l.CreatedAt, UpdatedAt: l.UpdatedAt,
	}
}

type nodeResponse struct {
	ID              string            `json:"id"`
	Name            string            `json:"name"`
	Protocol        string            `json:"protocol"`
	Address         string            `json:"address"`
	Port            int               `json:"port"`
	EndpointIP      string            `json:"endpoint_ip,omitempty"`
	Region          string            `json:"region,omitempty"`
	SourceURL       string            `json:"source_url,omitempty"`
	SourceType      string            `json:"source_type"`
	Status          domain.NodeStatus `json:"status"`
	ExitIP          string            `json:"exit_ip,omitempty"`
	Country         string            `json:"country,omitempty"`
	City            string            `json:"city,omitempty"`
	ASN             string            `json:"asn,omitempty"`
	Organization    string            `json:"organization,omitempty"`
	GeoSource       string            `json:"geo_source,omitempty"`
	GeoUpdatedAt    *time.Time        `json:"geo_updated_at,omitempty"`
	RegionChangedAt *time.Time        `json:"region_changed_at,omitempty"`
	Score           *scoreResponse    `json:"score,omitempty"`
	LatencyMS       *float64          `json:"latency_ms,omitempty"`
	SpeedBPS        *float64          `json:"speed_bytes_per_sec,omitempty"`
	CreatedAt       time.Time         `json:"created_at"`
	UpdatedAt       time.Time         `json:"updated_at"`
}

type scoreResponse struct {
	Latency             float64               `json:"latency"`
	Speed               float64               `json:"speed"`
	Availability        float64               `json:"availability"`
	Overall             float64               `json:"overall"`
	Status              domain.Recommendation `json:"status"`
	LatencySamples      int                   `json:"latency_samples"`
	SpeedSamples        int                   `json:"speed_samples"`
	AvailabilitySamples int                   `json:"availability_samples"`
	StrategyVersion     int                   `json:"strategy_version"`
	CalculatedAt        time.Time             `json:"calculated_at"`
}

func toNodeResponse(n *domain.Node) nodeResponse {
	nr := nodeResponse{
		ID: n.ID, Name: n.Name, Protocol: n.Protocol, Address: n.Address,
		Port: n.Port, EndpointIP: n.EndpointIP, ExitIP: n.ExitIP, Country: n.Country,
		City: n.City, ASN: n.ASN, Organization: n.Organization, Region: n.Region,
		GeoSource: n.GeoSource, GeoUpdatedAt: n.GeoUpdatedAt, RegionChangedAt: n.RegionChangedAt,
		SourceURL: n.SourceURL, Status: n.Status,
		SourceType: nodeSourceType(n),
		LatencyMS:  n.LastLatencyMS, SpeedBPS: n.LastSpeedBPS,
		CreatedAt: n.CreatedAt, UpdatedAt: n.UpdatedAt,
	}
	if n.Score != nil {
		s := &scoreResponse{
			Latency: n.Score.Latency, Speed: n.Score.Speed,
			Availability: n.Score.Availability, Overall: n.Score.Overall,
			Status: n.Score.Status, LatencySamples: n.Score.LatencySamples,
			SpeedSamples: n.Score.SpeedSamples, AvailabilitySamples: n.Score.AvailabilitySamples,
			StrategyVersion: n.Score.StrategyVersion, CalculatedAt: n.Score.CalculatedAt,
		}
		nr.Score = s
	}
	return nr
}

func nodeSourceType(n *domain.Node) string {
	if n.SourceURL == "" || n.SourceURL == "manual" {
		return "manual"
	}
	return "subscription"
}

func toNodeResponseList(nodes []domain.Node) []nodeResponse {
	result := make([]nodeResponse, 0, len(nodes))
	for i := range nodes {
		result = append(result, toNodeResponse(&nodes[i]))
	}
	return result
}

type nodeSourceResponse struct {
	NodeID       string    `json:"node_id"`
	UpstreamID   string    `json:"upstream_id"`
	OriginalName string    `json:"original_name"`
	RawFragment  string    `json:"raw_fragment,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

func toNodeSourceResponse(s domain.NodeSource) nodeSourceResponse {
	return nodeSourceResponse{
		NodeID: s.NodeID, UpstreamID: s.UpstreamID,
		OriginalName: s.OriginalName,
		RawFragment:  s.RawFragment,
		CreatedAt:    s.CreatedAt,
	}
}

type measurementResponse struct {
	ID                          string    `json:"id"`
	NodeID                      string    `json:"node_id"`
	Kind                        string    `json:"kind"`
	Success                     bool      `json:"success"`
	LatencyMS                   float64   `json:"latency_ms,omitempty"`
	FirstByteMS                 float64   `json:"first_byte_ms,omitempty"`
	SpeedBytesPerSec            float64   `json:"speed_bytes_per_sec,omitempty"`
	EffectiveDownloadDurationMS float64   `json:"effective_download_duration_ms,omitempty"`
	Bytes                       int64     `json:"bytes,omitempty"`
	UploadBytes                 int64     `json:"upload_bytes,omitempty"`
	ProxyProtocol               string    `json:"proxy_protocol,omitempty"`
	TestURL                     string    `json:"test_url,omitempty"`
	ExitIP                      string    `json:"exit_ip,omitempty"`
	BaselineTarget              string    `json:"baseline_target,omitempty"`
	SpeedSource                 string    `json:"speed_source,omitempty"`
	LoadStatus                  string    `json:"load_status,omitempty"`
	BackgroundUploadBPS         float64   `json:"background_upload_bps,omitempty"`
	BackgroundDownloadBPS       float64   `json:"background_download_bps,omitempty"`
	WANDownloadBefore           int64     `json:"wan_download_before,omitempty"`
	WANDownloadAfter            int64     `json:"wan_download_after,omitempty"`
	WANUploadBefore             int64     `json:"wan_upload_before,omitempty"`
	WANUploadAfter              int64     `json:"wan_upload_after,omitempty"`
	WANDownloadCapacityBPS      float64   `json:"wan_download_capacity_bps,omitempty"`
	WANUploadCapacityBPS        float64   `json:"wan_upload_capacity_bps,omitempty"`
	LoadThreshold               float64   `json:"load_threshold,omitempty"`
	LoadSampleDurationMS        int64     `json:"load_sample_duration_ms,omitempty"`
	ErrorCode                   string    `json:"error_code,omitempty"`
	FailureStage                string    `json:"failure_stage,omitempty"`
	Infrastructure              bool      `json:"infrastructure"`
	CoreEvidence                any       `json:"core_evidence,omitempty"`
	CreatedAt                   time.Time `json:"created_at"`
}

func toMeasurementResponse(m domain.Measurement) measurementResponse {
	return measurementResponse{
		ID: m.ID, NodeID: m.NodeID, Kind: m.Kind, Success: m.Success,
		LatencyMS: m.LatencyMS, FirstByteMS: m.FirstByteMS,
		SpeedBytesPerSec:            m.SpeedBytesPerSec,
		EffectiveDownloadDurationMS: m.EffectiveDownloadDurationMS,
		Bytes:                       m.Bytes, ErrorCode: m.ErrorCode, FailureStage: m.FailureStage,
		UploadBytes:   m.UploadBytes,
		ProxyProtocol: m.ProxyProtocol, TestURL: m.TestURL, ExitIP: m.ExitIP,
		BaselineTarget: m.BaselineTarget, SpeedSource: m.SpeedSource, LoadStatus: m.LoadStatus,
		BackgroundUploadBPS: m.BackgroundUploadBPS, BackgroundDownloadBPS: m.BackgroundDownloadBPS,
		WANDownloadBefore: m.WANDownloadBefore, WANDownloadAfter: m.WANDownloadAfter,
		WANUploadBefore: m.WANUploadBefore, WANUploadAfter: m.WANUploadAfter,
		WANDownloadCapacityBPS: m.WANDownloadCapacityBPS, WANUploadCapacityBPS: m.WANUploadCapacityBPS,
		LoadThreshold: m.LoadThreshold, LoadSampleDurationMS: m.LoadSampleDurationMS,
		Infrastructure: m.Infrastructure, CoreEvidence: m.CoreEvidence, CreatedAt: m.CreatedAt,
	}
}

type scoreSnapshotResponse struct {
	ID                  string                `json:"id"`
	NodeID              string                `json:"node_id"`
	Latency             float64               `json:"latency"`
	Speed               float64               `json:"speed"`
	Availability        float64               `json:"availability"`
	Overall             float64               `json:"overall"`
	Status              domain.Recommendation `json:"status"`
	LatencySamples      int                   `json:"latency_samples"`
	SpeedSamples        int                   `json:"speed_samples"`
	AvailabilitySamples int                   `json:"availability_samples"`
	StrategyVersion     int                   `json:"strategy_version"`
	CalculatedAt        time.Time             `json:"calculated_at"`
}

func toScoreSnapshotResponse(s *domain.ScoreSnapshot) scoreSnapshotResponse {
	return scoreSnapshotResponse{
		ID: s.ID, NodeID: s.NodeID, Latency: s.Latency, Speed: s.Speed,
		Availability: s.Availability, Overall: s.Overall, Status: s.Status,
		LatencySamples: s.LatencySamples, SpeedSamples: s.SpeedSamples,
		AvailabilitySamples: s.AvailabilitySamples,
		StrategyVersion:     s.StrategyVersion, CalculatedAt: s.CalculatedAt,
	}
}
