package domain

import "errors"

// ErrNotFound is returned by repositories when an entity is not found.
var ErrNotFound = errors.New("not found")

// Stable error codes per SDD 4.4
const (
	ErrCodeCoreUnavailable        = "core_unavailable"
	ErrCodeCoreAPIUnavailable     = "core_api_unavailable"
	ErrCodeNodeLoadFailed         = "node_load_failed"
	ErrCodeCoreRouteUnverified    = "core_route_unverified"
	ErrCodeBaselineUnavailable    = "network_baseline_unavailable"
	ErrCodeBaselineLost           = "network_baseline_lost"
	ErrCodeNetworkBusy            = "network_busy"
	ErrCodeNetworkContended       = "network_contended"
	ErrCodeNetworkLoadUnknown     = "network_load_unknown"
	ErrCodeSpeedSourceUnavailable = "speed_source_unavailable"
	ErrCodeMeasurementFailed      = "measurement_failed"
	ErrCodeNetworkUnreachable     = "network_unreachable"
	ErrCodeHostUnreachable        = "host_unreachable"
	ErrCodeEndpointUnreachable    = "endpoint_ip_unreachable"
)

// TestKind identifies the category of a measurement task per SDD 7.1-7.3.
type TestKind string

const (
	TestConnectivity TestKind = "connectivity"
	TestPing         TestKind = "ping"
	TestThroughput   TestKind = "throughput"
)

// MeasurementOutcome is the unified result for all three test kinds.
// Infrastructure failures (baseline / load-guard) are flagged with Infrastructure=true
// so they NEVER affect node scoring (SDD 7.4.2, 7.5.1).
type MeasurementOutcome struct {
	JobID                       string
	NodeID                      string
	Kind                        TestKind
	Status                      string
	Success                     bool
	ErrorCode                   string
	ErrorDetail                 string
	FailureStage                string
	Infrastructure              bool
	LatencyMS                   float64
	FirstByteMS                 float64
	SpeedBytesPerSec            float64
	DownloadBytes               int64
	UploadBytes                 int64
	EffectiveDownloadDurationMS float64
	ProxyProtocol               string
	TestURL                     string
	ExitIP                      string
	BaselineTarget              string
	SpeedSource                 string
	LoadStatus                  string
	BackgroundUploadBPS         float64
	BackgroundDownloadBPS       float64
	WANDownloadBefore           int64
	WANDownloadAfter            int64
	WANUploadBefore             int64
	WANUploadAfter              int64
	WANDownloadCapacityBPS      float64
	WANUploadCapacityBPS        float64
	LoadThreshold               float64
	LoadSampleDurationMS        int64
	Evidence                    CoreEvidence
}
