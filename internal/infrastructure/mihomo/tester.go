package mihomo

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/teddymail/bagualu/internal/domain"
)

// Tester is the only adapter used by measurement jobs. It serializes node
// selection, proxy traffic, and evidence collection so a shared Mihomo
// selector cannot be changed by another job.
type Tester struct {
	client       *Client
	proxyURL     string
	selector     string
	instanceID   string
	pid          func() int
	httpClient   *http.Client
	exitIPURL    string
	configDigest string
	mu           sync.Mutex
}

func NewTester(client *Client, proxyPort int, selector, instanceID string, pid func() int) (*Tester, error) {
	if client == nil || proxyPort < 1 || proxyPort > 65535 || selector == "" {
		return nil, fmt.Errorf("invalid mihomo tester configuration")
	}
	if pid == nil {
		pid = func() int { return 0 }
	}
	proxy, err := url.Parse("http://127.0.0.1:" + strconv.Itoa(proxyPort))
	if err != nil {
		return nil, err
	}
	proxyTransport := &http.Transport{Proxy: http.ProxyURL(proxy), MaxIdleConnsPerHost: 2}
	return &Tester{
		client: client, proxyURL: "http://127.0.0.1:" + strconv.Itoa(proxyPort),
		selector: selector, instanceID: instanceID, pid: pid,
		httpClient: &http.Client{Transport: proxyTransport, Timeout: 30 * time.Second},
		exitIPURL:  "https://api.ipify.org",
	}, nil
}

func (t *Tester) transport() (*http.Client, error) {
	if t.httpClient == nil {
		return nil, fmt.Errorf("%s: proxy client unavailable", domain.ErrCodeCoreUnavailable)
	}
	return t.httpClient, nil
}

func (t *Tester) SetConfigDigest(digest string) { t.configDigest = digest }

func (t *Tester) SetExitIPURL(value string) { t.exitIPURL = value }

func (t *Tester) prepare(ctx context.Context, nodeName string) (int64, int64, error) {
	if err := t.client.Health(ctx); err != nil {
		return 0, 0, fmt.Errorf("%s: %w", domain.ErrCodeCoreAPIUnavailable, err)
	}
	if _, err := t.client.GetProxy(ctx, nodeName); err != nil {
		return 0, 0, fmt.Errorf("%s: %w", domain.ErrCodeNodeLoadFailed, err)
	}
	if err := t.client.Select(ctx, t.selector, nodeName); err != nil {
		return 0, 0, fmt.Errorf("%s: %w", domain.ErrCodeCoreAPIUnavailable, err)
	}
	download, upload, err := t.client.TrafficTotals(ctx)
	if err != nil {
		return 0, 0, fmt.Errorf("%s: %w", errorCode(err), err)
	}
	return download, upload, nil
}

func (t *Tester) evidence(ctx context.Context, nodeName string, before, uploadBefore int64, pid int) (domain.CoreEvidence, error) {
	after, uploadAfter, err := t.client.TrafficTotals(ctx)
	if err != nil {
		return domain.CoreEvidence{}, fmt.Errorf("%s: %w", domain.ErrCodeCoreAPIUnavailable, err)
	}
	connections, err := t.client.GetConnections(ctx)
	if err != nil {
		return domain.CoreEvidence{}, fmt.Errorf("%s: %w", domain.ErrCodeCoreAPIUnavailable, err)
	}
	connection, ok := FindConnectionByNode(connections, nodeName)
	if !ok {
		return domain.CoreEvidence{}, fmt.Errorf("%s: no connection chain for %q", domain.ErrCodeCoreRouteUnverified, nodeName)
	}
	if after <= before {
		return domain.CoreEvidence{}, fmt.Errorf("%s: mihomo traffic did not increase", domain.ErrCodeCoreRouteUnverified)
	}
	version, err := t.client.Version(ctx)
	if err != nil {
		return domain.CoreEvidence{}, fmt.Errorf("%s: %w", domain.ErrCodeCoreAPIUnavailable, err)
	}
	exitIP := ""
	if t.exitIPURL != "" {
		request, requestErr := http.NewRequestWithContext(ctx, http.MethodGet, t.exitIPURL, nil)
		if requestErr == nil {
			response, requestErr := t.httpClient.Do(request)
			if requestErr == nil {
				data, readErr := io.ReadAll(io.LimitReader(response.Body, 128))
				response.Body.Close()
				if readErr == nil {
					if parsed := net.ParseIP(strings.TrimSpace(string(data))); parsed != nil {
						exitIP = parsed.String()
					}
				}
			}
		}
	}
	if t.exitIPURL != "" && exitIP == "" {
		return domain.CoreEvidence{}, fmt.Errorf("%s: proxy exit IP could not be verified", domain.ErrCodeCoreRouteUnverified)
	}
	return domain.CoreEvidence{
		PID: strconv.Itoa(pid), InstanceID: t.instanceID, Version: version,
		NodeName: nodeName, ConnectionID: connection.ID, ExitIP: exitIP,
		TrafficBefore: before, TrafficAfter: after,
		UploadBefore: uploadBefore, UploadAfter: uploadAfter,
		LogicalConnections: len(connections.Connections),
		ConfigDigest:       t.configDigest,
	}, nil
}

func (t *Tester) do(ctx context.Context, nodeName, target string, maxBytes int64, throughput bool) (domain.MeasurementOutcome, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	start := time.Now()
	pid := t.pid()
	before, uploadBefore, err := t.prepare(ctx, nodeName)
	if err != nil {
		return domain.MeasurementOutcome{Status: "failed", ErrorCode: errorCode(err), FailureStage: "core"}, err
	}
	httpClient, err := t.transport()
	if err != nil {
		return domain.MeasurementOutcome{Status: "failed", ErrorCode: errorCode(err), FailureStage: "proxy"}, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return domain.MeasurementOutcome{Status: "failed", ErrorCode: "measurement_failed", FailureStage: "request"}, err
	}
	response, err := httpClient.Do(request)
	if err != nil {
		return domain.MeasurementOutcome{Status: "failed", ErrorCode: networkErrorCode(err), FailureStage: "proxy_request", TestURL: target}, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 400 {
		code := domain.ErrCodeMeasurementFailed
		if throughput && response.StatusCode >= 500 {
			code = domain.ErrCodeSpeedSourceUnavailable
		}
		snippet, _ := io.ReadAll(io.LimitReader(response.Body, 512))
		detail := strings.Join(strings.Fields(string(snippet)), " ")
		if detail == "" {
			detail = http.StatusText(response.StatusCode)
		}
		err = fmt.Errorf("proxy request returned HTTP %d: %s", response.StatusCode, detail)
		return domain.MeasurementOutcome{Status: "failed", ErrorCode: code, FailureStage: "response", TestURL: target}, err
	}
	firstByte := time.Since(start)
	reader := io.Reader(response.Body)
	if maxBytes > 0 {
		reader = io.LimitReader(response.Body, maxBytes)
	}
	downloadStart := time.Now()
	bytesRead, err := io.Copy(io.Discard, reader)
	if err != nil {
		return domain.MeasurementOutcome{Status: "failed", ErrorCode: "measurement_failed", FailureStage: "download", TestURL: target}, err
	}
	downloadDuration := time.Since(downloadStart)
	evidence, err := t.evidence(ctx, nodeName, before, uploadBefore, pid)
	if err != nil {
		return domain.MeasurementOutcome{Status: "failed", ErrorCode: errorCode(err), FailureStage: "evidence"}, err
	}
	if downloadDuration <= 0 {
		downloadDuration = time.Nanosecond
	}
	return domain.MeasurementOutcome{
		Status: "succeeded", Success: true, LatencyMS: float64(firstByte.Milliseconds()),
		FirstByteMS: float64(firstByte.Milliseconds()), DownloadBytes: bytesRead,
		EffectiveDownloadDurationMS: float64(downloadDuration.Milliseconds()),
		SpeedBytesPerSec:            float64(bytesRead) / downloadDuration.Seconds(), TestURL: target,
		ExitIP: evidence.ExitIP, UploadBytes: evidence.UploadAfter - evidence.UploadBefore, Evidence: evidence,
	}, nil
}

func (t *Tester) Connectivity(ctx context.Context, nodeName, target string) (domain.MeasurementOutcome, error) {
	return t.do(ctx, nodeName, target, 1<<20, false)
}

func (t *Tester) Throughput(ctx context.Context, nodeName, target string, maxBytes int64) (domain.MeasurementOutcome, error) {
	return t.do(ctx, nodeName, target, maxBytes, true)
}

func errorCode(err error) string {
	for _, code := range []string{domain.ErrCodeCoreRouteUnverified, domain.ErrCodeNodeLoadFailed, domain.ErrCodeNetworkUnreachable, domain.ErrCodeHostUnreachable, domain.ErrCodeCoreAPIUnavailable, domain.ErrCodeCoreUnavailable} {
		if len(err.Error()) >= len(code) && containsCode(err.Error(), code) {
			return code
		}
	}
	return "measurement_failed"
}

func containsCode(message, code string) bool {
	for i := 0; i+len(code) <= len(message); i++ {
		if message[i:i+len(code)] == code {
			return true
		}
	}
	return false
}
