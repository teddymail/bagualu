package httptransport_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/teddymail/bagualu/internal/domain"
	"github.com/teddymail/bagualu/internal/infrastructure/persistence"
	httptransport "github.com/teddymail/bagualu/internal/transport/http"
)

// ── test helpers ──────────────────────────────────────────────────────────────

func newTestServer(t *testing.T) (*httptransport.Server, *persistence.Store) {
	t.Helper()
	store, err := persistence.Open(":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	srv := httptransport.NewServerWithConfig(httptransport.Config{
		CoreStatus: func() domain.CoreStatus {
			return domain.CoreStatus{Available: true, Version: "test"}
		},
		Store:         store,
		AdminPassword: "testpass",
	})
	return srv, store
}

func doRequest(t *testing.T, handler http.Handler, method, path string, body any, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	var bodyReader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		bodyReader = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, bodyReader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	return rr
}

func decodeBody(t *testing.T, rr *httptest.ResponseRecorder, v any) {
	t.Helper()
	if err := json.NewDecoder(rr.Body).Decode(v); err != nil {
		t.Fatalf("decode body: %v (status=%d body=%q)", err, rr.Code, rr.Body.String())
	}
}

// login returns a Bearer token for the given password.
func login(t *testing.T, handler http.Handler, password string) string {
	t.Helper()
	rr := doRequest(t, handler, "POST", "/api/v1/auth/login", map[string]string{"password": password}, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("login: expected 200 got %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	decodeBody(t, rr, &resp)
	tok, ok := resp["token"].(string)
	if !ok || tok == "" {
		t.Fatalf("login: no token in response")
	}
	return "Bearer " + tok
}

func authHeader(token string) map[string]string {
	return map[string]string{"Authorization": token}
}

// ── public endpoints ──────────────────────────────────────────────────────────

func TestHealth(t *testing.T) {
	srv, _ := newTestServer(t)
	rr := doRequest(t, srv.Handler(), "GET", "/api/v1/health", nil, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("want 200 got %d", rr.Code)
	}
	var resp map[string]string
	decodeBody(t, rr, &resp)
	if resp["status"] != "ok" {
		t.Fatalf("want status=ok got %q", resp["status"])
	}
}

func TestHealth_LegacyNewServer(t *testing.T) {
	// Verify that the old NewServer(status) constructor still works.
	srv := httptransport.NewServer(func() domain.CoreStatus {
		return domain.CoreStatus{Available: false}
	})
	rr := doRequest(t, srv.Handler(), "GET", "/api/v1/health", nil, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("want 200 got %d", rr.Code)
	}
}

// ── auth endpoints ────────────────────────────────────────────────────────────

func TestAuthLogin_Success(t *testing.T) {
	srv, _ := newTestServer(t)
	rr := doRequest(t, srv.Handler(), "POST", "/api/v1/auth/login", map[string]string{"password": "testpass"}, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("want 200 got %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	decodeBody(t, rr, &resp)
	if _, ok := resp["token"]; !ok {
		t.Fatal("missing token in response")
	}
}

func TestAuthLogin_WrongPassword(t *testing.T) {
	srv, _ := newTestServer(t)
	rr := doRequest(t, srv.Handler(), "POST", "/api/v1/auth/login", map[string]string{"password": "wrong"}, nil)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("want 401 got %d", rr.Code)
	}
}

func TestAuthLogin_MissingPassword(t *testing.T) {
	srv, _ := newTestServer(t)
	rr := doRequest(t, srv.Handler(), "POST", "/api/v1/auth/login", map[string]string{}, nil)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want 400 got %d", rr.Code)
	}
}

func TestAuthMe(t *testing.T) {
	srv, _ := newTestServer(t)
	token := login(t, srv.Handler(), "testpass")
	rr := doRequest(t, srv.Handler(), "GET", "/api/v1/auth/me", nil, authHeader(token))
	if rr.Code != http.StatusOK {
		t.Fatalf("want 200 got %d", rr.Code)
	}
}

func TestAuthMe_Unauthorized(t *testing.T) {
	srv, _ := newTestServer(t)
	rr := doRequest(t, srv.Handler(), "GET", "/api/v1/auth/me", nil, nil)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("want 401 got %d", rr.Code)
	}
}

func TestAuthLogout(t *testing.T) {
	srv, _ := newTestServer(t)
	token := login(t, srv.Handler(), "testpass")
	rr := doRequest(t, srv.Handler(), "POST", "/api/v1/auth/logout", nil, authHeader(token))
	if rr.Code != http.StatusOK {
		t.Fatalf("want 200 got %d", rr.Code)
	}
	// Token should be revoked now
	rr2 := doRequest(t, srv.Handler(), "GET", "/api/v1/auth/me", nil, authHeader(token))
	if rr2.Code != http.StatusUnauthorized {
		t.Fatalf("token should be revoked, want 401 got %d", rr2.Code)
	}
}

func TestAuthChangePassword(t *testing.T) {
	srv, _ := newTestServer(t)
	token := login(t, srv.Handler(), "testpass")
	rr := doRequest(t, srv.Handler(), "PUT", "/api/v1/auth/password",
		map[string]string{"old_password": "testpass", "new_password": "newpass"},
		authHeader(token))
	if rr.Code != http.StatusOK {
		t.Fatalf("want 200 got %d: %s", rr.Code, rr.Body.String())
	}
	// Old password should no longer work
	rr2 := doRequest(t, srv.Handler(), "POST", "/api/v1/auth/login",
		map[string]string{"password": "testpass"}, nil)
	if rr2.Code != http.StatusUnauthorized {
		t.Fatalf("old password should be rejected, want 401 got %d", rr2.Code)
	}
	// New password should work
	rr3 := doRequest(t, srv.Handler(), "POST", "/api/v1/auth/login",
		map[string]string{"password": "newpass"}, nil)
	if rr3.Code != http.StatusOK {
		t.Fatalf("new password should be accepted, want 200 got %d", rr3.Code)
	}
}

// ── system endpoints ──────────────────────────────────────────────────────────

func TestSystemStatus(t *testing.T) {
	srv, _ := newTestServer(t)
	token := login(t, srv.Handler(), "testpass")
	rr := doRequest(t, srv.Handler(), "GET", "/api/v1/system/status", nil, authHeader(token))
	if rr.Code != http.StatusOK {
		t.Fatalf("want 200 got %d", rr.Code)
	}
	var resp map[string]any
	decodeBody(t, rr, &resp)
	if resp["service"] != "running" {
		t.Fatalf("want service=running got %q", resp["service"])
	}
}

func TestCoreStatus(t *testing.T) {
	srv, _ := newTestServer(t)
	token := login(t, srv.Handler(), "testpass")
	rr := doRequest(t, srv.Handler(), "GET", "/api/v1/system/core/status", nil, authHeader(token))
	if rr.Code != http.StatusOK {
		t.Fatalf("want 200 got %d", rr.Code)
	}
}

func TestCoreInstallEndpoints(t *testing.T) {
	store, err := persistence.Open(":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	srv := httptransport.NewServerWithConfig(httptransport.Config{
		CoreStatus: func() domain.CoreStatus { return domain.CoreStatus{Available: false} },
		CoreInstallStatus: func(context.Context) domain.CoreInstallStatus {
			return domain.CoreInstallStatus{Installed: false, Path: "/usr/bin/mihomo", Architecture: "linux/amd64"}
		},
		CoreInstall: func(context.Context) (domain.CoreInstallResult, error) {
			return domain.CoreInstallResult{Version: "v1.2.3", Path: "/usr/bin/mihomo", Verified: true}, nil
		},
		Store:         store,
		AdminPassword: "testpass",
	})
	token := login(t, srv.Handler(), "testpass")
	status := doRequest(t, srv.Handler(), "GET", "/api/v1/system/core/install/status", nil, authHeader(token))
	if status.Code != http.StatusOK {
		t.Fatalf("status: want 200 got %d", status.Code)
	}
	install := doRequest(t, srv.Handler(), "POST", "/api/v1/system/core/install", nil, authHeader(token))
	if install.Code != http.StatusOK {
		t.Fatalf("install: want 200 got %d: %s", install.Code, install.Body.String())
	}
}

func TestRuntimeSummary(t *testing.T) {
	srv, _ := newTestServer(t)
	token := login(t, srv.Handler(), "testpass")
	rr := doRequest(t, srv.Handler(), "GET", "/api/v1/runtime/summary", nil, authHeader(token))
	if rr.Code != http.StatusOK {
		t.Fatalf("want 200 got %d", rr.Code)
	}
}

func TestDashboardSummary(t *testing.T) {
	srv, _ := newTestServer(t)
	token := login(t, srv.Handler(), "testpass")
	rr := doRequest(t, srv.Handler(), "GET", "/api/v1/dashboard/summary", nil, authHeader(token))
	if rr.Code != http.StatusOK {
		t.Fatalf("want 200 got %d", rr.Code)
	}
}

// ── upstream CRUD ─────────────────────────────────────────────────────────────

func TestUpstreamCRUD(t *testing.T) {
	srv, _ := newTestServer(t)
	token := login(t, srv.Handler(), "testpass")
	h := srv.Handler()

	// Create
	rr := doRequest(t, h, "POST", "/api/v1/upstreams", map[string]any{
		"name":    "test-upstream",
		"url":     "https://example.com/sub",
		"format":  "clash",
		"enabled": true,
	}, authHeader(token))
	if rr.Code != http.StatusCreated {
		t.Fatalf("create: want 201 got %d: %s", rr.Code, rr.Body.String())
	}
	var upstream map[string]any
	decodeBody(t, rr, &upstream)
	id := upstream["id"].(string)

	// List
	rr2 := doRequest(t, h, "GET", "/api/v1/upstreams", nil, authHeader(token))
	if rr2.Code != http.StatusOK {
		t.Fatalf("list: want 200 got %d", rr2.Code)
	}
	var listResp map[string]any
	decodeBody(t, rr2, &listResp)
	upstreams := listResp["upstreams"].([]any)
	if len(upstreams) != 1 {
		t.Fatalf("want 1 upstream got %d", len(upstreams))
	}

	// Get
	rr3 := doRequest(t, h, "GET", "/api/v1/upstreams/"+id, nil, authHeader(token))
	if rr3.Code != http.StatusOK {
		t.Fatalf("get: want 200 got %d", rr3.Code)
	}

	// Update
	rr4 := doRequest(t, h, "PUT", "/api/v1/upstreams/"+id, map[string]any{
		"name": "updated-upstream",
	}, authHeader(token))
	if rr4.Code != http.StatusOK {
		t.Fatalf("update: want 200 got %d: %s", rr4.Code, rr4.Body.String())
	}

	// Refresh
	rr5 := doRequest(t, h, "POST", "/api/v1/upstreams/"+id+"/refresh", nil, authHeader(token))
	if rr5.Code != http.StatusAccepted {
		t.Fatalf("refresh: want 202 got %d: %s", rr5.Code, rr5.Body.String())
	}

	// Delete
	rr6 := doRequest(t, h, "DELETE", "/api/v1/upstreams/"+id, nil, authHeader(token))
	if rr6.Code != http.StatusNoContent {
		t.Fatalf("delete: want 204 got %d", rr6.Code)
	}

	// Get after delete → 404
	rr7 := doRequest(t, h, "GET", "/api/v1/upstreams/"+id, nil, authHeader(token))
	if rr7.Code != http.StatusNotFound {
		t.Fatalf("get after delete: want 404 got %d", rr7.Code)
	}
}

func TestUpstream_CreateMissingFields(t *testing.T) {
	srv, _ := newTestServer(t)
	token := login(t, srv.Handler(), "testpass")
	rr := doRequest(t, srv.Handler(), "POST", "/api/v1/upstreams", map[string]any{
		"name": "no-url",
	}, authHeader(token))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want 400 got %d", rr.Code)
	}
}

func TestNodeTestQueueFullReturnsTooManyRequests(t *testing.T) {
	store, err := persistence.Open(":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	now := time.Now().UTC()
	if err := store.NodeRepo().Save(context.Background(), &domain.Node{
		ID: "node-queue-full", Name: "queue-full", Protocol: "vless", Address: "127.0.0.1", Port: 443,
		Status: domain.NodeActive, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("save node: %v", err)
	}
	srv := httptransport.NewServerWithConfig(httptransport.Config{
		CoreStatus:    func() domain.CoreStatus { return domain.CoreStatus{Available: true} },
		Store:         store,
		AdminPassword: "testpass",
		TestSubmit: func(context.Context, string, domain.TestKind) (string, error) {
			return "", errors.New("test_queue_full")
		},
	})
	token := login(t, srv.Handler(), "testpass")
	rr := doRequest(t, srv.Handler(), "POST", "/api/v1/nodes/node-queue-full/tests/throughput", nil, authHeader(token))
	if rr.Code != http.StatusTooManyRequests {
		t.Fatalf("want 429 got %d: %s", rr.Code, rr.Body.String())
	}
	var response map[string]any
	decodeBody(t, rr, &response)
	if response["error_code"] != "test_queue_full" {
		t.Fatalf("unexpected error response: %v", response)
	}
}

func TestUpstreamTestQueueFullReturnsPartialAccepted(t *testing.T) {
	store, err := persistence.Open(":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	now := time.Now().UTC()
	if err := store.UpstreamRepo().Save(context.Background(), &domain.Upstream{
		ID: "upstream-queue-full", Name: "queue-full", URL: "https://example.com/sub", Enabled: true,
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("save upstream: %v", err)
	}
	for _, id := range []string{"node-accepted", "node-skipped"} {
		if err := store.NodeRepo().Save(context.Background(), &domain.Node{
			ID: id, Name: id, Protocol: "vless", Address: "127.0.0.1", Port: 443,
			SourceURL: "https://example.com/sub", Status: domain.NodeActive, CreatedAt: now, UpdatedAt: now,
		}); err != nil {
			t.Fatalf("save node: %v", err)
		}
	}
	submitCount := 0
	srv := httptransport.NewServerWithConfig(httptransport.Config{
		CoreStatus:    func() domain.CoreStatus { return domain.CoreStatus{Available: true} },
		Store:         store,
		AdminPassword: "testpass",
		TestSubmit: func(context.Context, string, domain.TestKind) (string, error) {
			submitCount++
			if submitCount == 1 {
				return "job-accepted", nil
			}
			return "", errors.New("test_queue_full")
		},
	})
	token := login(t, srv.Handler(), "testpass")
	rr := doRequest(t, srv.Handler(), "POST", "/api/v1/upstreams/upstream-queue-full/tests/throughput", nil, authHeader(token))
	if rr.Code != http.StatusAccepted {
		t.Fatalf("want 202 got %d: %s", rr.Code, rr.Body.String())
	}
	var response struct {
		NodeCount    int                 `json:"node_count"`
		QueuedCount  int                 `json:"queued_count"`
		SkippedCount int                 `json:"skipped_count"`
		JobIDs       []string            `json:"job_ids"`
		Skipped      []map[string]string `json:"skipped"`
	}
	decodeBody(t, rr, &response)
	if response.NodeCount != 1 || response.QueuedCount != 1 || response.SkippedCount != 1 || len(response.JobIDs) != 1 || len(response.Skipped) != 1 {
		t.Fatalf("unexpected partial response: %+v", response)
	}
}

// ── group CRUD ────────────────────────────────────────────────────────────────

func TestGroupCRUD(t *testing.T) {
	srv, _ := newTestServer(t)
	token := login(t, srv.Handler(), "testpass")
	h := srv.Handler()

	// Create
	rr := doRequest(t, h, "POST", "/api/v1/groups", map[string]any{
		"name":        "test-group",
		"description": "a test group",
		"min_score":   70.0,
	}, authHeader(token))
	if rr.Code != http.StatusCreated {
		t.Fatalf("create: want 201 got %d: %s", rr.Code, rr.Body.String())
	}
	var group map[string]any
	decodeBody(t, rr, &group)
	id := group["id"].(string)

	// List
	rr2 := doRequest(t, h, "GET", "/api/v1/groups", nil, authHeader(token))
	if rr2.Code != http.StatusOK {
		t.Fatalf("list: want 200 got %d", rr2.Code)
	}
	var listResp map[string]any
	decodeBody(t, rr2, &listResp)
	groups := listResp["groups"].([]any)
	if len(groups) != 1 {
		t.Fatalf("want 1 group got %d", len(groups))
	}

	// Get
	rr3 := doRequest(t, h, "GET", "/api/v1/groups/"+id, nil, authHeader(token))
	if rr3.Code != http.StatusOK {
		t.Fatalf("get: want 200 got %d", rr3.Code)
	}

	// Update
	newName := "updated-group"
	rr4 := doRequest(t, h, "PUT", "/api/v1/groups/"+id, map[string]any{
		"name": newName,
	}, authHeader(token))
	if rr4.Code != http.StatusOK {
		t.Fatalf("update: want 200 got %d: %s", rr4.Code, rr4.Body.String())
	}
	var updated map[string]any
	decodeBody(t, rr4, &updated)
	if updated["name"] != newName {
		t.Fatalf("want name=%q got %q", newName, updated["name"])
	}

	// Delete
	rr5 := doRequest(t, h, "DELETE", "/api/v1/groups/"+id, nil, authHeader(token))
	if rr5.Code != http.StatusNoContent {
		t.Fatalf("delete: want 204 got %d", rr5.Code)
	}
}

// ── job endpoints ─────────────────────────────────────────────────────────────

func TestJobListAndGet(t *testing.T) {
	srv, store := newTestServer(t)
	token := login(t, srv.Handler(), "testpass")
	h := srv.Handler()

	// Seed a job directly
	now := time.Now().UTC()
	job := &domain.Job{
		ID:        "job-test-1",
		Kind:      "refresh_upstream",
		Status:    domain.JobPending,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := store.JobRepo().Save(context.Background(), job); err != nil {
		t.Fatalf("seed job: %v", err)
	}
	if err := store.JobRepo().Save(context.Background(), &domain.Job{ID: "job-done", Kind: "test_ping", Status: domain.JobSucceeded, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("seed inactive job: %v", err)
	}
	if _, err := store.DB.Exec(`INSERT INTO jobs(id,kind,status,created_at,updated_at) VALUES(?,?,?,?,?)`, "job-unknown", "test_ping", "unknown", now.UTC().Format(time.RFC3339Nano), now.UTC().Format(time.RFC3339Nano)); err != nil {
		t.Fatalf("seed unknown job: %v", err)
	}

	// List
	rr := doRequest(t, h, "GET", "/api/v1/jobs", nil, authHeader(token))
	if rr.Code != http.StatusOK {
		t.Fatalf("list: want 200 got %d", rr.Code)
	}
	var listResp map[string]any
	decodeBody(t, rr, &listResp)
	jobs := listResp["jobs"].([]any)
	if len(jobs) != 1 {
		t.Fatalf("want only active job got %d", len(jobs))
	}
	if jobs[0].(map[string]any)["id"] != job.ID || jobs[0].(map[string]any)["status"] != string(domain.JobPending) {
		t.Fatalf("unexpected active job list: %+v", jobs)
	}

	// Get
	rr2 := doRequest(t, h, "GET", "/api/v1/jobs/job-test-1", nil, authHeader(token))
	if rr2.Code != http.StatusOK {
		t.Fatalf("get: want 200 got %d", rr2.Code)
	}

	// Cancel
	rr3 := doRequest(t, h, "POST", "/api/v1/jobs/job-test-1/cancel", nil, authHeader(token))
	if rr3.Code != http.StatusOK {
		t.Fatalf("cancel: want 200 got %d: %s", rr3.Code, rr3.Body.String())
	}

	// Cancel again → 409
	rr4 := doRequest(t, h, "POST", "/api/v1/jobs/job-test-1/cancel", nil, authHeader(token))
	if rr4.Code != http.StatusConflict {
		t.Fatalf("second cancel: want 409 got %d", rr4.Code)
	}
}

func TestClearActiveTestJobsPreservesHistoryAndOtherJobs(t *testing.T) {
	srv, store := newTestServer(t)
	token := login(t, srv.Handler(), "testpass")
	now := time.Now().UTC()
	for _, job := range []*domain.Job{
		{ID: "test-pending", Kind: "test_ping", Status: domain.JobPending, CreatedAt: now, UpdatedAt: now},
		{ID: "test-scheduled", Kind: "test_throughput", Status: domain.JobScheduled, CreatedAt: now, UpdatedAt: now},
		{ID: "refresh-pending", Kind: "refresh_upstream", Status: domain.JobPending, CreatedAt: now, UpdatedAt: now},
		{ID: "test-done", Kind: "test_ping", Status: domain.JobSucceeded, CreatedAt: now, UpdatedAt: now},
	} {
		if err := store.JobRepo().Save(context.Background(), job); err != nil {
			t.Fatalf("seed job %s: %v", job.ID, err)
		}
	}

	rr := doRequest(t, srv.Handler(), "POST", "/api/v1/jobs/clear", nil, authHeader(token))
	if rr.Code != http.StatusOK {
		t.Fatalf("clear: want 200 got %d: %s", rr.Code, rr.Body.String())
	}
	var response map[string]any
	decodeBody(t, rr, &response)
	if response["requested"] != float64(2) || response["cancelled"] != float64(2) {
		t.Fatalf("unexpected clear response: %+v", response)
	}
	for _, id := range []string{"test-pending", "test-scheduled"} {
		job, err := store.JobRepo().FindByID(context.Background(), id)
		if err != nil || job.Status != domain.JobCancelled {
			t.Fatalf("job %s should be cancelled, got %+v err=%v", id, job, err)
		}
	}
	refresh, err := store.JobRepo().FindByID(context.Background(), "refresh-pending")
	if err != nil || refresh.Status != domain.JobPending {
		t.Fatalf("refresh job should remain pending, got %+v err=%v", refresh, err)
	}
	done, err := store.JobRepo().FindByID(context.Background(), "test-done")
	if err != nil || done.Status != domain.JobSucceeded {
		t.Fatalf("completed job should remain succeeded, got %+v err=%v", done, err)
	}
}

func TestClearJobsPurgeDeletesAllTaskData(t *testing.T) {
	srv, store := newTestServer(t)
	token := login(t, srv.Handler(), "testpass")
	now := time.Now().UTC()
	for _, job := range []*domain.Job{
		{ID: "purge-pending", Kind: "test_ping", Status: domain.JobPending, CreatedAt: now, UpdatedAt: now},
		{ID: "purge-done", Kind: "test_ping", Status: domain.JobSucceeded, CreatedAt: now, UpdatedAt: now},
	} {
		if err := store.JobRepo().Save(context.Background(), job); err != nil {
			t.Fatalf("seed job %s: %v", job.ID, err)
		}
	}
	rr := doRequest(t, srv.Handler(), "POST", "/api/v1/jobs/clear?scope=all&purge=true", nil, authHeader(token))
	if rr.Code != http.StatusOK {
		t.Fatalf("purge: want 200 got %d: %s", rr.Code, rr.Body.String())
	}
	var response map[string]any
	decodeBody(t, rr, &response)
	if response["purged"] != true {
		t.Fatalf("expected purged response: %+v", response)
	}
	jobs, err := store.JobRepo().FindAll(context.Background(), domain.JobFilter{})
	if err != nil || len(jobs) != 0 {
		t.Fatalf("task data was not purged: %d jobs, err=%v", len(jobs), err)
	}
}
