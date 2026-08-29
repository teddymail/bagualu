package application

import (
	"context"
	"net"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"sync"
	"time"

	"github.com/teddymail/bagualu/internal/domain"
)

var icmpReplyPattern = regexp.MustCompile(`time[=<]([0-9]+(?:\.[0-9]+)?)\s*ms`)

// ICMPBaseline uses the router's default network namespace and never uses the
// Mihomo proxy. OpenWrt supplies the ping binary used here.
type ICMPBaseline struct {
	Target  string
	Count   int
	Timeout time.Duration
}

func (b ICMPBaseline) Check(ctx context.Context) BaselineResult {
	target := b.Target
	if target == "" {
		target = "www.baidu.com"
	}
	hosts, err := net.LookupHost(target)
	if err != nil {
		return BaselineResult{Target: target, ErrorCode: "network_baseline_unavailable"}
	}
	_ = hosts
	count := b.Count
	if count < 1 {
		count = 3
	}
	timeout := b.Timeout
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	timeoutValue := strconv.Itoa(int(timeout.Seconds()))
	if runtime.GOOS == "darwin" {
		timeoutValue = strconv.Itoa(int(timeout.Milliseconds()))
	}
	args := []string{"-c", strconv.Itoa(count), "-W", timeoutValue, target}
	output, commandErr := exec.CommandContext(ctx, "ping", args...).CombinedOutput()
	received := len(icmpReplyPattern.FindAllString(string(output), -1))
	if commandErr != nil && received == 0 {
		return BaselineResult{Target: target, Probes: count, Received: received, ErrorCode: "network_baseline_unavailable"}
	}
	return BaselineResult{Available: received >= 1, Target: target, Probes: count, Received: received}
}

type FixedLoadGuard struct {
	Status LoadStatus
}

type TrafficSample struct{ DownloadBytes, UploadBytes int64 }

type BandwidthLoadGuard struct {
	Reader              func(context.Context) (TrafficSample, error)
	DownloadCapacityBPS float64
	UploadCapacityBPS   float64
	Threshold           float64
	SampleWindow        time.Duration
	Config              func() (downloadCapacityBPS, uploadCapacityBPS, threshold float64)
	state               *bandwidthLoadState
}

type bandwidthLoadState struct {
	mu                         sync.Mutex
	start                      TrafficSample
	started                    time.Time
	capacity                   float64
	downAllowance, upAllowance float64
	downloadCapacity           float64
	uploadCapacity             float64
	threshold                  float64
}

func NewBandwidthLoadGuard(reader func(context.Context) (TrafficSample, error), config func() (float64, float64, float64)) *BandwidthLoadGuard {
	return &BandwidthLoadGuard{Reader: reader, Config: config, state: &bandwidthLoadState{}}
}

func (g BandwidthLoadGuard) Check(ctx context.Context) LoadResult {
	downloadCapacity, uploadCapacity, threshold := g.DownloadCapacityBPS, g.UploadCapacityBPS, g.Threshold
	if g.Config != nil {
		downloadCapacity, uploadCapacity, threshold = g.Config()
	}
	if g.Reader == nil || downloadCapacity <= 0 || uploadCapacity <= 0 {
		return LoadResult{Status: LoadUnknown}
	}
	if threshold <= 0 {
		threshold = 0.10
	}
	window := g.SampleWindow
	if window <= 0 {
		window = 5 * time.Second
	}
	first, err := g.Reader(ctx)
	if err != nil {
		return LoadResult{Status: LoadUnknown}
	}
	timer := time.NewTimer(window)
	select {
	case <-ctx.Done():
		timer.Stop()
		return LoadResult{Status: LoadUnknown}
	case <-timer.C:
	}
	second, err := g.Reader(ctx)
	if err != nil {
		return LoadResult{Status: LoadUnknown}
	}
	download := float64(second.DownloadBytes-first.DownloadBytes) / window.Seconds()
	upload := float64(second.UploadBytes-first.UploadBytes) / window.Seconds()
	if download < 0 || upload < 0 {
		return LoadResult{Status: LoadUnknown}
	}
	result := LoadResult{Status: LoadClean, DownloadBps: download, UploadBps: upload,
		WANDownloadBefore: first.DownloadBytes, WANDownloadAfter: second.DownloadBytes,
		WANUploadBefore: first.UploadBytes, WANUploadAfter: second.UploadBytes,
		DownloadCapacityBPS: downloadCapacity, UploadCapacityBPS: uploadCapacity,
		Threshold: threshold, SampleDurationMS: window.Milliseconds()}
	if download/downloadCapacity > threshold || upload/uploadCapacity > threshold {
		result.Status = LoadBusy
	}
	if g.state != nil {
		g.state.mu.Lock()
		g.state.start = second
		g.state.started = time.Now()
		g.state.downAllowance = downloadCapacity * threshold
		g.state.upAllowance = uploadCapacity * threshold
		g.state.capacity = g.state.downAllowance
		g.state.downloadCapacity = downloadCapacity
		g.state.uploadCapacity = uploadCapacity
		g.state.threshold = threshold
		g.state.mu.Unlock()
	}
	return result
}

// CheckDuring classifies background traffic observed after a throughput test.
// The task's own downloaded bytes are subtracted before comparing the remainder.
func (g BandwidthLoadGuard) CheckDuring(ctx context.Context, outcome domain.MeasurementOutcome) LoadResult {
	if g.Reader == nil || g.state == nil {
		return LoadResult{Status: LoadUnknown}
	}
	end, err := g.Reader(ctx)
	if err != nil {
		return LoadResult{Status: LoadUnknown}
	}
	g.state.mu.Lock()
	start, started, downAllowance, upAllowance := g.state.start, g.state.started, g.state.downAllowance, g.state.upAllowance
	downloadCapacity, uploadCapacity, threshold := g.state.downloadCapacity, g.state.uploadCapacity, g.state.threshold
	if downAllowance <= 0 {
		downAllowance = g.state.capacity
	}
	g.state.mu.Unlock()
	if started.IsZero() || downAllowance <= 0 {
		return LoadResult{Status: LoadUnknown}
	}
	if upAllowance <= 0 {
		upAllowance = 1<<63 - 1
	}
	elapsed := time.Since(started).Seconds()
	if elapsed <= 0 {
		return LoadResult{Status: LoadUnknown}
	}
	downloadBackground := float64(end.DownloadBytes-start.DownloadBytes-outcome.DownloadBytes) / elapsed
	if downloadBackground < 0 {
		downloadBackground = 0
	}
	uploadBackground := float64(end.UploadBytes-start.UploadBytes-outcome.UploadBytes) / elapsed
	if uploadBackground < 0 {
		uploadBackground = 0
	}
	result := LoadResult{Status: LoadClean, DownloadBps: downloadBackground, UploadBps: uploadBackground,
		WANDownloadBefore: start.DownloadBytes, WANDownloadAfter: end.DownloadBytes,
		WANUploadBefore: start.UploadBytes, WANUploadAfter: end.UploadBytes,
		DownloadCapacityBPS: downloadCapacity, UploadCapacityBPS: uploadCapacity,
		Threshold: threshold, SampleDurationMS: int64(elapsed * 1000)}
	if downloadBackground > downAllowance || uploadBackground > upAllowance {
		result.Status = LoadContended
	}
	return result
}

func (g FixedLoadGuard) Check(context.Context) LoadResult {
	if g.Status == "" {
		return LoadResult{Status: LoadUnknown}
	}
	return LoadResult{Status: g.Status}
}
