package mihomo

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/teddymail/bagualu/internal/domain"
)

// sharedTransport is reused for all Client instances to avoid creating
// new connection pools per request (SDD 4.4.1).
var sharedTransport = &http.Transport{
	MaxIdleConnsPerHost: 4,
	MaxConnsPerHost:     4,
	IdleConnTimeout:     30 * time.Second,
}

// Client calls the Mihomo external-controller HTTP API.
// All methods map HTTP / network errors to stable domain error codes.
type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

// NewClient creates a Client that connects to the Mihomo control API at controlURL,
// authenticated with token.
func NewClient(controlURL, token string) *Client {
	return &Client{
		baseURL: strings.TrimRight(controlURL, "/"),
		token:   token,
		httpClient: &http.Client{
			Transport: sharedTransport,
			Timeout:   10 * time.Second,
		},
	}
}

// request performs an HTTP call, applies auth, limits response size, and decodes JSON.
// Network errors and non-2xx responses are mapped to KernelError with a stable code.
func (c *Client) request(ctx context.Context, method, path string, body io.Reader, target any) error {
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return kernelErr(domain.ErrCodeCoreAPIUnavailable, fmt.Errorf("build request: %w", err))
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return kernelErr(networkErrorCode(err), err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
		ke := &KernelError{
			Code:       mapHTTPStatus(resp.StatusCode),
			HTTPStatus: resp.StatusCode,
			Cause:      fmt.Errorf("http %d: %s", resp.StatusCode, strings.TrimSpace(string(snippet))),
		}
		return ke
	}

	if target == nil {
		return nil
	}
	limited := io.LimitReader(resp.Body, 1<<20) // 1 MB cap
	if err := json.NewDecoder(limited).Decode(target); err != nil {
		return kernelErr(domain.ErrCodeCoreAPIUnavailable, fmt.Errorf("decode response: %w", err))
	}
	return nil
}

func networkErrorCode(err error) string {
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "network is unreachable") || strings.Contains(message, "no route to host") {
		return domain.ErrCodeNetworkUnreachable
	}
	if strings.Contains(message, "host is down") || strings.Contains(message, "host unreachable") {
		return domain.ErrCodeHostUnreachable
	}
	return domain.ErrCodeCoreAPIUnavailable
}

// Health checks the Mihomo process is reachable (GET /).
func (c *Client) Health(ctx context.Context) error {
	return c.request(ctx, http.MethodGet, "/", nil, nil)
}

// Version returns the Mihomo version string (GET /version).
func (c *Client) Version(ctx context.Context) (string, error) {
	var resp struct {
		Version string `json:"version"`
	}
	if err := c.request(ctx, http.MethodGet, "/version", nil, &resp); err != nil {
		return "", err
	}
	if resp.Version == "" {
		return "", kernelErr(domain.ErrCodeCoreAPIUnavailable, fmt.Errorf("version field missing"))
	}
	return resp.Version, nil
}

// Select sets the active proxy in a selector group (PUT /proxies/{group}).
func (c *Client) Select(ctx context.Context, group, node string) error {
	payload := fmt.Sprintf(`{"name":%q}`, node)
	return c.request(ctx, http.MethodPut, "/proxies/"+url.PathEscape(group),
		strings.NewReader(payload), nil)
}

// ProxyHistory is a single delay sample returned by Mihomo.
type ProxyHistory struct {
	Time  string `json:"time"`
	Delay int    `json:"delay"`
}

// ProxyInfo is a proxy/node as reported by the Mihomo /proxies API.
type ProxyInfo struct {
	Name    string         `json:"name"`
	Type    string         `json:"type"`
	History []ProxyHistory `json:"history"`
	Now     string         `json:"now,omitempty"` // for selector groups
}

// GetProxies returns all proxies known to Mihomo (GET /proxies).
func (c *Client) GetProxies(ctx context.Context) (map[string]ProxyInfo, error) {
	var resp struct {
		Proxies map[string]ProxyInfo `json:"proxies"`
	}
	if err := c.request(ctx, http.MethodGet, "/proxies", nil, &resp); err != nil {
		return nil, err
	}
	return resp.Proxies, nil
}

// GetProxy returns a single proxy by name (GET /proxies/{name}).
// Returns ErrCodeNodeLoadFailed if the proxy is not found.
func (c *Client) GetProxy(ctx context.Context, name string) (ProxyInfo, error) {
	var info ProxyInfo
	if err := c.request(ctx, http.MethodGet, "/proxies/"+url.PathEscape(name), nil, &info); err != nil {
		if ke, ok := err.(*KernelError); ok && ke.HTTPStatus == http.StatusNotFound {
			ke.Code = domain.ErrCodeNodeLoadFailed
		}
		return ProxyInfo{}, err
	}
	return info, nil
}

// Delay calls Mihomo's built-in URL delay test for a named proxy.
// Returns latency in milliseconds (GET /proxies/{name}/delay).
func (c *Client) Delay(ctx context.Context, name, testURL string, timeoutMS int) (int64, error) {
	query := url.Values{
		"url":     {testURL},
		"timeout": {fmt.Sprintf("%d", timeoutMS)},
	}
	path := "/proxies/" + url.PathEscape(name) + "/delay?" + query.Encode()
	var resp struct {
		Delay int64 `json:"delay"`
	}
	if err := c.request(ctx, http.MethodGet, path, nil, &resp); err != nil {
		return 0, err
	}
	return resp.Delay, nil
}

// ConnectionEntry represents one active connection from GET /connections.
type ConnectionEntry struct {
	ID       string   `json:"id"`
	Chains   []string `json:"chains"`
	Upload   int64    `json:"upload"`
	Download int64    `json:"download"`
	Start    string   `json:"start"`
}

// ConnectionsSnapshot is the full response from GET /connections.
type ConnectionsSnapshot struct {
	DownloadTotal int64             `json:"downloadTotal"`
	UploadTotal   int64             `json:"uploadTotal"`
	Connections   []ConnectionEntry `json:"connections"`
}

// GetConnections returns typed connection evidence from Mihomo (GET /connections).
func (c *Client) GetConnections(ctx context.Context) (ConnectionsSnapshot, error) {
	var snap ConnectionsSnapshot
	if err := c.request(ctx, http.MethodGet, "/connections", nil, &snap); err != nil {
		return ConnectionsSnapshot{}, err
	}
	return snap, nil
}

// TrafficTotals returns cumulative download/upload byte counts from the connections endpoint.
// Use snapshots before and after a test to compute traffic delta (SDD 4.4 step 5, 7).
func (c *Client) TrafficTotals(ctx context.Context) (download, upload int64, err error) {
	snap, err := c.GetConnections(ctx)
	if err != nil {
		return 0, 0, err
	}
	return snap.DownloadTotal, snap.UploadTotal, nil
}

// FindConnectionByNode scans a ConnectionsSnapshot for an entry whose chain contains nodeName.
// Returns (entry, true) if found.
func FindConnectionByNode(snap ConnectionsSnapshot, nodeName string) (ConnectionEntry, bool) {
	for _, conn := range snap.Connections {
		for _, chain := range conn.Chains {
			if chain == nodeName {
				return conn, true
			}
		}
	}
	return ConnectionEntry{}, false
}

// LoadConfig tells Mihomo to load configuration from configPath (PUT /configs).
// Returns ErrCodeNodeLoadFailed on 400/422 (invalid config), ErrCodeCoreAPIUnavailable otherwise.
func (c *Client) LoadConfig(ctx context.Context, configPath string) error {
	payload, err := json.Marshal(map[string]string{"path": configPath})
	if err != nil {
		return kernelErr(domain.ErrCodeCoreAPIUnavailable, err)
	}
	if err := c.request(ctx, http.MethodPut, "/configs", bytes.NewReader(payload), nil); err != nil {
		if ke, ok := err.(*KernelError); ok &&
			(ke.HTTPStatus == http.StatusBadRequest || ke.HTTPStatus == http.StatusUnprocessableEntity) {
			ke.Code = domain.ErrCodeNodeLoadFailed
		}
		return err
	}
	return nil
}

// ReloadConfig forces Mihomo to reload the current configuration file (PUT /configs?force=true).
func (c *Client) ReloadConfig(ctx context.Context) error {
	return c.request(ctx, http.MethodPut, "/configs?force=true", strings.NewReader("{}"), nil)
}

// Connections returns raw connection data (kept for backward compat; prefer GetConnections).
func (c *Client) Connections(ctx context.Context) (any, error) {
	snap, err := c.GetConnections(ctx)
	if err != nil {
		return nil, err
	}
	return snap, nil
}

// Traffic returns raw traffic data (GET /traffic). Kept for backward compat.
func (c *Client) Traffic(ctx context.Context) (map[string]int64, error) {
	var response map[string]int64
	if err := c.request(ctx, http.MethodGet, "/traffic", nil, &response); err != nil {
		return nil, err
	}
	return response, nil
}

type Status struct{ domain.CoreStatus }

// Status returns the CoreStatus for the current Mihomo instance.
func (c *Client) Status(ctx context.Context) (domain.CoreStatus, error) {
	version, err := c.Version(ctx)
	if err != nil {
		return domain.CoreStatus{Control: c.baseURL, ErrorCode: domain.ErrCodeCoreAPIUnavailable}, err
	}
	return domain.CoreStatus{Available: true, Version: version, Control: c.baseURL}, nil
}
