package mihomo

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/teddymail/bagualu/internal/domain"
)

func TestErrorCode(t *testing.T) {
	if got := ErrorCode(nil); got != "" {
		t.Fatalf("expected empty error code for nil, got %q", got)
	}
	if got := ErrorCode(errors.New("plain")); got != "" {
		t.Fatalf("expected empty error code for plain error, got %q", got)
	}
	err := kernelErr(domain.ErrCodeCoreUnavailable, errors.New("boom"))
	if got := ErrorCode(err); got != domain.ErrCodeCoreUnavailable {
		t.Fatalf("expected %q, got %q", domain.ErrCodeCoreUnavailable, got)
	}
}

func TestClientHealthSuccess(t *testing.T) {
	var method, path, auth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method, path, auth = r.Method, r.URL.Path, r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewClient(server.URL, "token")
	err := client.Health(context.Background())
	if err != nil {
		t.Fatalf("Health() error = %v", err)
	}
	if got := ErrorCode(err); got != "" {
		t.Fatalf("expected empty error code, got %q", got)
	}
	if method != http.MethodGet || path != "/" {
		t.Fatalf("unexpected request %s %s", method, path)
	}
	if auth != "Bearer token" {
		t.Fatalf("unexpected auth header %q", auth)
	}
}

func TestClientHealthFailure(t *testing.T) {
	var method, path string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method, path = r.Method, r.URL.Path
		http.Error(w, "down", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	client := NewClient(server.URL, "token")
	err := client.Health(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	if got := ErrorCode(err); got != domain.ErrCodeCoreAPIUnavailable {
		t.Fatalf("expected %q, got %q", domain.ErrCodeCoreAPIUnavailable, got)
	}
	if method != http.MethodGet || path != "/" {
		t.Fatalf("unexpected request %s %s", method, path)
	}
}

func TestClientVersion(t *testing.T) {
	var method, path string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method, path = r.Method, r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"version":"1.2.3"}`)
	}))
	defer server.Close()

	client := NewClient(server.URL, "token")
	version, err := client.Version(context.Background())
	if err != nil {
		t.Fatalf("Version() error = %v", err)
	}
	if got := ErrorCode(err); got != "" {
		t.Fatalf("expected empty error code, got %q", got)
	}
	if version != "1.2.3" {
		t.Fatalf("expected version 1.2.3, got %q", version)
	}
	if method != http.MethodGet || path != "/version" {
		t.Fatalf("unexpected request %s %s", method, path)
	}
}

func TestClientVersionUnauthorized(t *testing.T) {
	var method, path string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method, path = r.Method, r.URL.Path
		http.Error(w, "nope", http.StatusUnauthorized)
	}))
	defer server.Close()

	client := NewClient(server.URL, "token")
	_, err := client.Version(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	if got := ErrorCode(err); got != domain.ErrCodeCoreAPIUnavailable {
		t.Fatalf("expected %q, got %q", domain.ErrCodeCoreAPIUnavailable, got)
	}
	if method != http.MethodGet || path != "/version" {
		t.Fatalf("unexpected request %s %s", method, path)
	}
}

func TestClientSelect(t *testing.T) {
	var method, path, body string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method, path = r.Method, r.URL.Path
		payload, _ := io.ReadAll(r.Body)
		body = string(payload)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := NewClient(server.URL, "token")
	err := client.Select(context.Background(), "Selector", "Node A")
	if err != nil {
		t.Fatalf("Select() error = %v", err)
	}
	if got := ErrorCode(err); got != "" {
		t.Fatalf("expected empty error code, got %q", got)
	}
	if method != http.MethodPut || path != "/proxies/Selector" {
		t.Fatalf("unexpected request %s %s", method, path)
	}
	if body != `{"name":"Node A"}` {
		t.Fatalf("unexpected body %q", body)
	}
}

func TestClientGetProxy(t *testing.T) {
	var method, path string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method, path = r.Method, r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"name":"Node A","type":"ss","history":[{"time":"now","delay":12}]}`)
	}))
	defer server.Close()

	client := NewClient(server.URL, "token")
	info, err := client.GetProxy(context.Background(), "Node A")
	if err != nil {
		t.Fatalf("GetProxy() error = %v", err)
	}
	if got := ErrorCode(err); got != "" {
		t.Fatalf("expected empty error code, got %q", got)
	}
	if method != http.MethodGet || path != "/proxies/Node A" {
		t.Fatalf("unexpected request %s %s", method, path)
	}
	if info.Name != "Node A" || info.Type != "ss" || len(info.History) != 1 || info.History[0].Delay != 12 {
		t.Fatalf("unexpected proxy info %+v", info)
	}
}

func TestClientGetProxyNotFound(t *testing.T) {
	var method, path string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method, path = r.Method, r.URL.Path
		http.Error(w, "missing", http.StatusNotFound)
	}))
	defer server.Close()

	client := NewClient(server.URL, "token")
	_, err := client.GetProxy(context.Background(), "missing")
	if err == nil {
		t.Fatal("expected error")
	}
	if got := ErrorCode(err); got != domain.ErrCodeNodeLoadFailed {
		t.Fatalf("expected %q, got %q", domain.ErrCodeNodeLoadFailed, got)
	}
	if method != http.MethodGet || path != "/proxies/missing" {
		t.Fatalf("unexpected request %s %s", method, path)
	}
}

func TestClientGetProxies(t *testing.T) {
	var method, path string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method, path = r.Method, r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"proxies":{"Node A":{"name":"Node A","type":"ss"},"Selector":{"name":"Selector","type":"Selector","now":"Node A"}}}`)
	}))
	defer server.Close()

	client := NewClient(server.URL, "token")
	proxies, err := client.GetProxies(context.Background())
	if err != nil {
		t.Fatalf("GetProxies() error = %v", err)
	}
	if got := ErrorCode(err); got != "" {
		t.Fatalf("expected empty error code, got %q", got)
	}
	if method != http.MethodGet || path != "/proxies" {
		t.Fatalf("unexpected request %s %s", method, path)
	}
	if len(proxies) != 2 || proxies["Node A"].Type != "ss" || proxies["Selector"].Now != "Node A" {
		t.Fatalf("unexpected proxies %+v", proxies)
	}
}

func TestClientGetConnections(t *testing.T) {
	var method, path string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method, path = r.Method, r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"downloadTotal":123,"uploadTotal":45,"connections":[{"id":"c1","chains":["Node A","DIRECT"],"upload":4,"download":7,"start":"now"}]}`)
	}))
	defer server.Close()

	client := NewClient(server.URL, "token")
	snap, err := client.GetConnections(context.Background())
	if err != nil {
		t.Fatalf("GetConnections() error = %v", err)
	}
	if got := ErrorCode(err); got != "" {
		t.Fatalf("expected empty error code, got %q", got)
	}
	if method != http.MethodGet || path != "/connections" {
		t.Fatalf("unexpected request %s %s", method, path)
	}
	if snap.DownloadTotal != 123 || snap.UploadTotal != 45 || len(snap.Connections) != 1 || snap.Connections[0].ID != "c1" {
		t.Fatalf("unexpected connections snapshot %+v", snap)
	}
}

func TestClientTrafficTotals(t *testing.T) {
	var method, path string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method, path = r.Method, r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"downloadTotal":222,"uploadTotal":111,"connections":[]}`)
	}))
	defer server.Close()

	client := NewClient(server.URL, "token")
	download, upload, err := client.TrafficTotals(context.Background())
	if err != nil {
		t.Fatalf("TrafficTotals() error = %v", err)
	}
	if got := ErrorCode(err); got != "" {
		t.Fatalf("expected empty error code, got %q", got)
	}
	if method != http.MethodGet || path != "/connections" {
		t.Fatalf("unexpected request %s %s", method, path)
	}
	if download != 222 || upload != 111 {
		t.Fatalf("unexpected totals download=%d upload=%d", download, upload)
	}
}

func TestFindConnectionByNode(t *testing.T) {
	snap := ConnectionsSnapshot{Connections: []ConnectionEntry{{ID: "c1", Chains: []string{"foo", "bar"}}, {ID: "c2", Chains: []string{"node-a", "baz"}}}}
	entry, ok := FindConnectionByNode(snap, "node-a")
	if !ok {
		t.Fatal("expected connection to be found")
	}
	if entry.ID != "c2" {
		t.Fatalf("expected c2, got %+v", entry)
	}
	if _, ok := FindConnectionByNode(snap, "missing"); ok {
		t.Fatal("expected missing node not to be found")
	}
}

func TestClientLoadConfig(t *testing.T) {
	var method, path, body string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method, path = r.Method, r.URL.Path
		payload, _ := io.ReadAll(r.Body)
		body = string(payload)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := NewClient(server.URL, "token")
	err := client.LoadConfig(context.Background(), "/path/to/config.yaml")
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if got := ErrorCode(err); got != "" {
		t.Fatalf("expected empty error code, got %q", got)
	}
	if method != http.MethodPut || path != "/configs" {
		t.Fatalf("unexpected request %s %s", method, path)
	}
	var decoded map[string]string
	if err := json.Unmarshal([]byte(body), &decoded); err != nil {
		t.Fatalf("failed to decode body: %v", err)
	}
	if decoded["path"] != "/path/to/config.yaml" {
		t.Fatalf("unexpected body %+v", decoded)
	}
}

func TestClientReloadConfig(t *testing.T) {
	var method, path, rawQuery, body string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method, path, rawQuery = r.Method, r.URL.Path, r.URL.RawQuery
		payload, _ := io.ReadAll(r.Body)
		body = string(payload)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := NewClient(server.URL, "token")
	err := client.ReloadConfig(context.Background())
	if err != nil {
		t.Fatalf("ReloadConfig() error = %v", err)
	}
	if got := ErrorCode(err); got != "" {
		t.Fatalf("expected empty error code, got %q", got)
	}
	if method != http.MethodPut || path != "/configs" || rawQuery != "force=true" {
		t.Fatalf("unexpected request %s %s?%s", method, path, rawQuery)
	}
	if body != "{}" {
		t.Fatalf("unexpected body %q", body)
	}
}

func TestClientDelay(t *testing.T) {
	var method, path, testURL, timeout string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method, path = r.Method, r.URL.Path
		testURL = r.URL.Query().Get("url")
		timeout = r.URL.Query().Get("timeout")
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"delay":345}`)
	}))
	defer server.Close()

	client := NewClient(server.URL, "token")
	delay, err := client.Delay(context.Background(), "Node A", "https://example.com/ping", 5000)
	if err != nil {
		t.Fatalf("Delay() error = %v", err)
	}
	if got := ErrorCode(err); got != "" {
		t.Fatalf("expected empty error code, got %q", got)
	}
	if method != http.MethodGet || path != "/proxies/Node A/delay" {
		t.Fatalf("unexpected request %s %s", method, path)
	}
	if testURL != "https://example.com/ping" || timeout != "5000" {
		t.Fatalf("unexpected query url=%q timeout=%q", testURL, timeout)
	}
	if delay != 345 {
		t.Fatalf("expected delay 345, got %d", delay)
	}
}

func TestClientLoadConfigValidationFailureCode(t *testing.T) {
	var method, path string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method, path = r.Method, r.URL.Path
		http.Error(w, "bad config", http.StatusBadRequest)
	}))
	defer server.Close()

	client := NewClient(server.URL, "token")
	err := client.LoadConfig(context.Background(), "/bad")
	if err == nil {
		t.Fatal("expected error")
	}
	if got := ErrorCode(err); got != domain.ErrCodeNodeLoadFailed {
		t.Fatalf("expected %q, got %q", domain.ErrCodeNodeLoadFailed, got)
	}
	if method != http.MethodPut || path != "/configs" {
		t.Fatalf("unexpected request %s %s", method, path)
	}
}
