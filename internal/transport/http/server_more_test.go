package httptransport_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/teddymail/bagualu/internal/domain"
)

func TestJob_NotFound(t *testing.T) {
	srv, _ := newTestServer(t)
	token := login(t, srv.Handler(), "testpass")
	rr := doRequest(t, srv.Handler(), "GET", "/api/v1/jobs/nonexistent", nil, authHeader(token))
	if rr.Code != http.StatusNotFound {
		t.Fatalf("want 404 got %d", rr.Code)
	}
}

// ── API key CRUD ──────────────────────────────────────────────────────────────

func TestAPIKeyCRUD(t *testing.T) {
	srv, _ := newTestServer(t)
	token := login(t, srv.Handler(), "testpass")
	h := srv.Handler()

	// Create group first (API key needs group)
	grr := doRequest(t, h, "POST", "/api/v1/groups", map[string]any{
		"name": "key-group",
	}, authHeader(token))
	if grr.Code != http.StatusCreated {
		t.Fatalf("create group: want 201 got %d", grr.Code)
	}
	var gResp map[string]any
	decodeBody(t, grr, &gResp)
	groupID := gResp["id"].(string)

	// Create key
	rr := doRequest(t, h, "POST", "/api/v1/api-keys", map[string]any{
		"name":     "test-key",
		"group_id": groupID,
	}, authHeader(token))
	if rr.Code != http.StatusCreated {
		t.Fatalf("create: want 201 got %d: %s", rr.Code, rr.Body.String())
	}
	var createResp map[string]any
	decodeBody(t, rr, &createResp)
	plainKey := createResp["key"].(string)
	if !strings.HasPrefix(plainKey, "bg_") {
		t.Fatalf("key should start with bg_: %q", plainKey)
	}
	keyMap := createResp["api_key"].(map[string]any)
	keyID := keyMap["id"].(string)

	// List
	rr2 := doRequest(t, h, "GET", "/api/v1/api-keys", nil, authHeader(token))
	if rr2.Code != http.StatusOK {
		t.Fatalf("list: want 200 got %d", rr2.Code)
	}

	// Update
	rr3 := doRequest(t, h, "PUT", "/api/v1/api-keys/"+keyID, map[string]any{
		"name": "renamed-key",
	}, authHeader(token))
	if rr3.Code != http.StatusOK {
		t.Fatalf("update: want 200 got %d: %s", rr3.Code, rr3.Body.String())
	}

	// Rotate
	rr4 := doRequest(t, h, "POST", "/api/v1/api-keys/"+keyID+"/rotate", nil, authHeader(token))
	if rr4.Code != http.StatusOK {
		t.Fatalf("rotate: want 200 got %d: %s", rr4.Code, rr4.Body.String())
	}
	var rotateResp map[string]any
	decodeBody(t, rr4, &rotateResp)
	newKey := rotateResp["key"].(string)
	if newKey == plainKey {
		t.Fatal("rotated key should differ from old key")
	}

	// Old key should no longer authenticate resource requests
	rr5 := doRequest(t, h, "GET", "/api/v1/resources", nil, authHeader("Bearer "+plainKey))
	if rr5.Code != http.StatusUnauthorized {
		t.Fatalf("old key: want 401 got %d", rr5.Code)
	}

	// New key should work for resource auth (group has no nodes → 422 no compatible nodes)
	rr6 := doRequest(t, h, "GET", "/api/v1/resources", nil, authHeader("Bearer "+newKey))
	// Expecting 200, 404, or 422 (no compatible nodes in empty group)
	if rr6.Code != http.StatusOK && rr6.Code != http.StatusNotFound && rr6.Code != http.StatusUnprocessableEntity {
		t.Fatalf("new key: want 200, 404 or 422 got %d: %s", rr6.Code, rr6.Body.String())
	}

	// Revoke
	rr7 := doRequest(t, h, "POST", "/api/v1/api-keys/"+keyID+"/revoke", nil, authHeader(token))
	if rr7.Code != http.StatusOK {
		t.Fatalf("revoke: want 200 got %d", rr7.Code)
	}

	// Revoked key should not authenticate
	rr8 := doRequest(t, h, "GET", "/api/v1/resources", nil, authHeader("Bearer "+newKey))
	if rr8.Code != http.StatusUnauthorized {
		t.Fatalf("revoked key: want 401 got %d", rr8.Code)
	}
}

// ── subscription link CRUD ────────────────────────────────────────────────────

func TestSubscriptionLinkCRUD(t *testing.T) {
	srv, _ := newTestServer(t)
	token := login(t, srv.Handler(), "testpass")
	h := srv.Handler()

	// Create group first
	grr := doRequest(t, h, "POST", "/api/v1/groups", map[string]any{
		"name": "sub-group",
	}, authHeader(token))
	if grr.Code != http.StatusCreated {
		t.Fatalf("create group: want 201 got %d", grr.Code)
	}
	var gResp map[string]any
	decodeBody(t, grr, &gResp)
	groupID := gResp["id"].(string)

	// Create link
	rr := doRequest(t, h, "POST", "/api/v1/subscription-links", map[string]any{
		"name":           "test-link",
		"group_id":       groupID,
		"default_format": "clash",
		"enabled":        true,
	}, authHeader(token))
	if rr.Code != http.StatusCreated {
		t.Fatalf("create: want 201 got %d: %s", rr.Code, rr.Body.String())
	}
	var createResp map[string]any
	decodeBody(t, rr, &createResp)
	plainToken := createResp["token"].(string)
	if !strings.HasPrefix(plainToken, "sl_") {
		t.Fatalf("token should start with sl_: %q", plainToken)
	}
	linkMap := createResp["subscription_link"].(map[string]any)
	linkID := linkMap["id"].(string)

	// List
	rr2 := doRequest(t, h, "GET", "/api/v1/subscription-links", nil, authHeader(token))
	if rr2.Code != http.StatusOK {
		t.Fatalf("list: want 200 got %d", rr2.Code)
	}

	// Get
	rr3 := doRequest(t, h, "GET", "/api/v1/subscription-links/"+linkID, nil, authHeader(token))
	if rr3.Code != http.StatusOK {
		t.Fatalf("get: want 200 got %d", rr3.Code)
	}

	// Update
	rr4 := doRequest(t, h, "PUT", "/api/v1/subscription-links/"+linkID, map[string]any{
		"name": "updated-link",
	}, authHeader(token))
	if rr4.Code != http.StatusOK {
		t.Fatalf("update: want 200 got %d: %s", rr4.Code, rr4.Body.String())
	}

	// Preview
	rr5 := doRequest(t, h, "GET", "/api/v1/subscription-links/"+linkID+"/preview", nil, authHeader(token))
	if rr5.Code != http.StatusOK {
		t.Fatalf("preview: want 200 got %d: %s", rr5.Code, rr5.Body.String())
	}

	// Detect preview
	rr6 := doRequest(t, h, "POST", "/api/v1/subscription-links/"+linkID+"/detect-preview",
		map[string]string{"user_agent": "ClashVerge/1.0"},
		authHeader(token))
	if rr6.Code != http.StatusOK {
		t.Fatalf("detect-preview: want 200 got %d", rr6.Code)
	}
	var dpResp map[string]any
	decodeBody(t, rr6, &dpResp)
	if dpResp["format"] != "clash" {
		t.Fatalf("detect-preview: want format=clash got %q", dpResp["format"])
	}

	// Rotate token
	rr7 := doRequest(t, h, "POST", "/api/v1/subscription-links/"+linkID+"/rotate", nil, authHeader(token))
	if rr7.Code != http.StatusOK {
		t.Fatalf("rotate: want 200 got %d: %s", rr7.Code, rr7.Body.String())
	}
	var rotateResp map[string]any
	decodeBody(t, rr7, &rotateResp)
	newToken := rotateResp["token"].(string)
	if newToken == plainToken {
		t.Fatal("rotated token should differ from old token")
	}

	// Old token should be rejected
	oldRR := doRequest(t, h, "GET", "/api/v1/subscribe/"+plainToken, nil, nil)
	if oldRR.Code != http.StatusUnauthorized {
		t.Fatalf("old token: want 401 got %d", oldRR.Code)
	}

	// Delete
	rr8 := doRequest(t, h, "DELETE", "/api/v1/subscription-links/"+linkID, nil, authHeader(token))
	if rr8.Code != http.StatusNoContent {
		t.Fatalf("delete: want 204 got %d", rr8.Code)
	}
}

// ── subscribe by token ────────────────────────────────────────────────────────

func TestSubscribeByToken_NoCompatibleNodes(t *testing.T) {
	srv, _ := newTestServer(t)
	token := login(t, srv.Handler(), "testpass")
	h := srv.Handler()

	// Create group + link (empty group → no compatible nodes)
	grr := doRequest(t, h, "POST", "/api/v1/groups", map[string]any{"name": "empty"}, authHeader(token))
	var gResp map[string]any
	decodeBody(t, grr, &gResp)
	groupID := gResp["id"].(string)

	lrr := doRequest(t, h, "POST", "/api/v1/subscription-links", map[string]any{
		"name": "empty-link", "group_id": groupID, "default_format": "clash", "enabled": true,
	}, authHeader(token))
	var lResp map[string]any
	decodeBody(t, lrr, &lResp)
	subToken := lResp["token"].(string)

	rr := doRequest(t, h, "GET", "/api/v1/subscribe/"+subToken, nil, nil)
	// No nodes → 422 with error code
	if rr.Code != http.StatusUnprocessableEntity {
		t.Fatalf("want 422 got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestSubscribeByToken_UADetection(t *testing.T) {
	srv, _ := newTestServer(t)
	token := login(t, srv.Handler(), "testpass")
	h := srv.Handler()

	grr := doRequest(t, h, "POST", "/api/v1/groups", map[string]any{"name": "ua-group"}, authHeader(token))
	var gResp map[string]any
	decodeBody(t, grr, &gResp)
	groupID := gResp["id"].(string)

	lrr := doRequest(t, h, "POST", "/api/v1/subscription-links", map[string]any{
		"name": "ua-link", "group_id": groupID, "default_format": "clash", "enabled": true,
	}, authHeader(token))
	var lResp map[string]any
	decodeBody(t, lrr, &lResp)
	subToken := lResp["token"].(string)

	// Make request with dae-wing UA – should set X-Bagualu-Format-Source: user-agent
	req := httptest.NewRequest("GET", "/api/v1/subscribe/"+subToken, nil)
	req.Header.Set("User-Agent", "dae-wing/1.0")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Header().Get("X-Bagualu-Format-Source") != "user-agent" {
		t.Fatalf("want X-Bagualu-Format-Source=user-agent got %q", rr.Header().Get("X-Bagualu-Format-Source"))
	}
	if rr.Header().Get("X-Bagualu-Subscription-Format") != "dae" {
		t.Fatalf("want X-Bagualu-Subscription-Format=dae got %q", rr.Header().Get("X-Bagualu-Subscription-Format"))
	}
	if rr.Header().Get("Vary") != "User-Agent" {
		t.Fatalf("want Vary=User-Agent got %q", rr.Header().Get("Vary"))
	}
}

func TestSubscribeByToken_ExplicitFormatOverridesUA(t *testing.T) {
	srv, _ := newTestServer(t)
	token := login(t, srv.Handler(), "testpass")
	h := srv.Handler()

	grr := doRequest(t, h, "POST", "/api/v1/groups", map[string]any{"name": "fmt-group"}, authHeader(token))
	var gResp map[string]any
	decodeBody(t, grr, &gResp)
	groupID := gResp["id"].(string)

	lrr := doRequest(t, h, "POST", "/api/v1/subscription-links", map[string]any{
		"name": "fmt-link", "group_id": groupID, "default_format": "base64", "enabled": true,
	}, authHeader(token))
	var lResp map[string]any
	decodeBody(t, lrr, &lResp)
	subToken := lResp["token"].(string)

	// Explicit ?format=clash even though UA would match dae-wing
	req := httptest.NewRequest("GET", "/api/v1/subscribe/"+subToken+"?format=clash", nil)
	req.Header.Set("User-Agent", "dae-wing/1.0")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Header().Get("X-Bagualu-Format-Source") != "query" {
		t.Fatalf("want X-Bagualu-Format-Source=query got %q", rr.Header().Get("X-Bagualu-Format-Source"))
	}
	if rr.Header().Get("X-Bagualu-Subscription-Format") != "clash" {
		t.Fatalf("want X-Bagualu-Subscription-Format=clash got %q", rr.Header().Get("X-Bagualu-Subscription-Format"))
	}
}

// ── score policy ──────────────────────────────────────────────────────────────

func TestScorePolicy(t *testing.T) {
	srv, _ := newTestServer(t)
	token := login(t, srv.Handler(), "testpass")
	h := srv.Handler()

	rr := doRequest(t, h, "GET", "/api/v1/score-policy", nil, authHeader(token))
	if rr.Code != http.StatusOK {
		t.Fatalf("want 200 got %d", rr.Code)
	}
	var resp map[string]any
	decodeBody(t, rr, &resp)
	if resp["recommendation_threshold"] == nil && resp["RecommendationThreshold"] == nil {
		t.Fatal("missing recommendation_threshold in score policy response")
	}
}

// ── regions ───────────────────────────────────────────────────────────────────

func TestRegions(t *testing.T) {
	srv, _ := newTestServer(t)
	token := login(t, srv.Handler(), "testpass")
	h := srv.Handler()

	rr := doRequest(t, h, "GET", "/api/v1/regions", nil, authHeader(token))
	if rr.Code != http.StatusOK {
		t.Fatalf("want 200 got %d", rr.Code)
	}
}

// ── resource API (API key auth) ────────────────────────────────────────────────

func TestResourcesRequireAPIKey(t *testing.T) {
	srv, _ := newTestServer(t)
	rr := doRequest(t, srv.Handler(), "GET", "/api/v1/resources", nil, nil)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("want 401 got %d", rr.Code)
	}
}

func TestResourcesWithValidKey(t *testing.T) {
	srv, _ := newTestServer(t)
	token := login(t, srv.Handler(), "testpass")
	h := srv.Handler()

	// Create group + API key
	grr := doRequest(t, h, "POST", "/api/v1/groups", map[string]any{"name": "res-group"}, authHeader(token))
	var gResp map[string]any
	decodeBody(t, grr, &gResp)
	groupID := gResp["id"].(string)

	krr := doRequest(t, h, "POST", "/api/v1/api-keys", map[string]any{
		"name": "res-key", "group_id": groupID,
	}, authHeader(token))
	var kResp map[string]any
	decodeBody(t, krr, &kResp)
	plainKey := kResp["key"].(string)

	// /resources → default group (empty group has no compatible nodes → 422 expected)
	rr := doRequest(t, h, "GET", "/api/v1/resources", nil, authHeader("Bearer "+plainKey))
	if rr.Code != http.StatusOK && rr.Code != http.StatusNotFound && rr.Code != http.StatusUnprocessableEntity {
		t.Fatalf("want 200, 404 or 422 got %d: %s", rr.Code, rr.Body.String())
	}

	// /resources/{group}
	rr2 := doRequest(t, h, "GET", "/api/v1/resources/"+groupID, nil, authHeader("Bearer "+plainKey))
	// No nodes → 422, with nodes → 200
	if rr2.Code != http.StatusUnprocessableEntity && rr2.Code != http.StatusOK {
		t.Fatalf("want 200 or 422 got %d: %s", rr2.Code, rr2.Body.String())
	}

	// /resources/{group}/nodes
	rr3 := doRequest(t, h, "GET", "/api/v1/resources/"+groupID+"/nodes", nil, authHeader("Bearer "+plainKey))
	if rr3.Code != http.StatusOK {
		t.Fatalf("nodes: want 200 got %d", rr3.Code)
	}

	// /resources/{group}/stats
	rr4 := doRequest(t, h, "GET", "/api/v1/resources/"+groupID+"/stats", nil, authHeader("Bearer "+plainKey))
	if rr4.Code != http.StatusOK {
		t.Fatalf("stats: want 200 got %d", rr4.Code)
	}
}

// ── client detection rules ────────────────────────────────────────────────────

func TestClientDetectionRules(t *testing.T) {
	srv, _ := newTestServer(t)
	token := login(t, srv.Handler(), "testpass")
	h := srv.Handler()

	rr := doRequest(t, h, "GET", "/api/v1/client-detection-rules", nil, authHeader(token))
	if rr.Code != http.StatusOK {
		t.Fatalf("want 200 got %d", rr.Code)
	}
	var resp map[string]any
	decodeBody(t, rr, &resp)
	if len(resp["rules"].([]any)) == 0 {
		t.Fatal("expected at least one detection rule")
	}
}

// ── test policy ───────────────────────────────────────────────────────────────

func TestTestPolicy(t *testing.T) {
	srv, _ := newTestServer(t)
	token := login(t, srv.Handler(), "testpass")
	h := srv.Handler()

	rr := doRequest(t, h, "GET", "/api/v1/test-policy", nil, authHeader(token))
	if rr.Code != http.StatusOK {
		t.Fatalf("want 200 got %d", rr.Code)
	}
	var policy map[string]any
	decodeBody(t, rr, &policy)
	if policy["interval_seconds"] != float64(60) {
		t.Fatalf("default ping interval = %v", policy["interval_seconds"])
	}
}

// ── 404 catch-all ─────────────────────────────────────────────────────────────

func TestUnknownEndpointReturns404(t *testing.T) {
	srv, _ := newTestServer(t)
	rr := doRequest(t, srv.Handler(), "GET", "/api/v1/does-not-exist", nil, nil)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("want 404 got %d", rr.Code)
	}
	var resp map[string]string
	decodeBody(t, rr, &resp)
	if resp["error_code"] != "not_found" {
		t.Fatalf("want error_code=not_found got %q", resp["error_code"])
	}
}

// ── error response structure ──────────────────────────────────────────────────

func TestErrorResponseHasErrorCodeAndMessage(t *testing.T) {
	srv, _ := newTestServer(t)
	rr := doRequest(t, srv.Handler(), "GET", "/api/v1/auth/me", nil, nil)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("want 401 got %d", rr.Code)
	}
	var resp map[string]string
	decodeBody(t, rr, &resp)
	if resp["error_code"] == "" {
		t.Fatal("error response must contain error_code")
	}
	if resp["message"] == "" {
		t.Fatal("error response must contain message")
	}
}

// ── content type headers ──────────────────────────────────────────────────────

func TestJSONResponsesHaveCorrectContentType(t *testing.T) {
	srv, _ := newTestServer(t)
	rr := doRequest(t, srv.Handler(), "GET", "/api/v1/health", nil, nil)
	ct := rr.Header().Get("Content-Type")
	if !strings.Contains(ct, "application/json") {
		t.Fatalf("want application/json content-type got %q", ct)
	}
}

// ── node endpoints ────────────────────────────────────────────────────────────

func TestNodeEnableDisable(t *testing.T) {
	srv, store := newTestServer(t)
	token := login(t, srv.Handler(), "testpass")
	h := srv.Handler()

	// Seed a node
	now := time.Now().UTC()
	node := &domain.Node{
		ID: "node-test-1", Name: "test-node", Protocol: "ss",
		Address: "1.2.3.4", Port: 8388, Status: domain.NodeActive,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := store.NodeRepo().Save(context.Background(), node); err != nil {
		t.Fatalf("seed node: %v", err)
	}

	// List nodes
	rr := doRequest(t, h, "GET", "/api/v1/nodes", nil, authHeader(token))
	if rr.Code != http.StatusOK {
		t.Fatalf("list: want 200 got %d", rr.Code)
	}

	// Get node
	rr2 := doRequest(t, h, "GET", "/api/v1/nodes/node-test-1", nil, authHeader(token))
	if rr2.Code != http.StatusOK {
		t.Fatalf("get: want 200 got %d", rr2.Code)
	}

	// Disable
	rr3 := doRequest(t, h, "POST", "/api/v1/nodes/node-test-1/disable", nil, authHeader(token))
	if rr3.Code != http.StatusOK {
		t.Fatalf("disable: want 200 got %d: %s", rr3.Code, rr3.Body.String())
	}

	// Enable
	rr4 := doRequest(t, h, "POST", "/api/v1/nodes/node-test-1/enable", nil, authHeader(token))
	if rr4.Code != http.StatusOK {
		t.Fatalf("enable: want 200 got %d", rr4.Code)
	}

	// Update
	rr5 := doRequest(t, h, "PUT", "/api/v1/nodes/node-test-1", map[string]any{
		"name": "renamed-node", "region": "US",
	}, authHeader(token))
	if rr5.Code != http.StatusOK {
		t.Fatalf("update: want 200 got %d: %s", rr5.Code, rr5.Body.String())
	}

	// Measurements
	rr6 := doRequest(t, h, "GET", "/api/v1/nodes/node-test-1/measurements", nil, authHeader(token))
	if rr6.Code != http.StatusOK {
		t.Fatalf("measurements: want 200 got %d", rr6.Code)
	}

	// Delete
	rr7 := doRequest(t, h, "DELETE", "/api/v1/nodes/node-test-1", nil, authHeader(token))
	if rr7.Code != http.StatusNoContent {
		t.Fatalf("delete: want 204 got %d", rr7.Code)
	}
}

func TestCreateNodeFromSingleURI(t *testing.T) {
	srv, store := newTestServer(t)
	token := login(t, srv.Handler(), "testpass")
	uri := "vless://11111111-1111-1111-1111-111111111111@example.com:443?encryption=none&security=tls&sni=example.com&type=tcp#manual-node"
	rr := doRequest(t, srv.Handler(), "POST", "/api/v1/nodes", map[string]any{
		"name": "我的单节点", "uri": uri,
	}, authHeader(token))
	if rr.Code != http.StatusCreated {
		t.Fatalf("create node: want 201 got %d: %s", rr.Code, rr.Body.String())
	}
	var response map[string]any
	decodeBody(t, rr, &response)
	if response["name"] != "我的单节点" || response["protocol"] != "vless" || response["address"] != "example.com" {
		t.Fatalf("unexpected node response: %+v", response)
	}
	if response["source_type"] != "manual" || response["source_url"] != "manual" {
		t.Fatalf("created node must be manual: %+v", response)
	}
	id, ok := response["id"].(string)
	if !ok || id == "" {
		t.Fatal("created node id is missing")
	}
	node, err := store.NodeRepo().FindByID(context.Background(), id)
	if err != nil || node.RawConfig["username"] != "11111111-1111-1111-1111-111111111111" {
		t.Fatalf("created node was not persisted correctly: %+v err=%v", node, err)
	}
	escapedURI := `vless\://c4215181-8e81-4eea-b71f-17605c9a8570\@160.16.140.60:443?type=tcp&encryption=none&security=reality&flow=xtls-rprx-vision&fp=chrome&insecure=0&sni=jp.charmnap.com&pbk=CIuFVwMetC8kuCvi7\_buV\_w-HStNrcPfMKpwK-68LGI&sid=8ca03e8a#%E6%97%A5%E6%9C%AC-8%E5%8F%B7`
	escaped := doRequest(t, srv.Handler(), "POST", "/api/v1/nodes", map[string]any{"uri": escapedURI}, authHeader(token))
	if escaped.Code != http.StatusCreated {
		t.Fatalf("create escaped VLESS node: want 201 got %d: %s", escaped.Code, escaped.Body.String())
	}
	var escapedResponse map[string]any
	decodeBody(t, escaped, &escapedResponse)
	if escapedResponse["name"] != "日本-8号" || escapedResponse["protocol"] != "vless" || escapedResponse["source_type"] != "manual" {
		t.Fatalf("unexpected escaped VLESS response: %+v", escapedResponse)
	}
}

func TestManualNodeCanBeUpdatedAndSubscriptionNodeIsReadOnly(t *testing.T) {
	srv, store := newTestServer(t)
	token := login(t, srv.Handler(), "testpass")
	manualURI := "socks5://example.com:1080#manual"
	created := doRequest(t, srv.Handler(), "POST", "/api/v1/nodes", map[string]any{"name": "manual", "uri": manualURI}, authHeader(token))
	if created.Code != http.StatusCreated {
		t.Fatalf("create manual node: want 201 got %d: %s", created.Code, created.Body.String())
	}
	var manualResponse map[string]any
	decodeBody(t, created, &manualResponse)
	manualID := manualResponse["id"].(string)
	updated := doRequest(t, srv.Handler(), "PUT", "/api/v1/nodes/"+manualID, map[string]any{
		"name": "updated", "uri": "socks5://example.net:1081#updated",
	}, authHeader(token))
	if updated.Code != http.StatusOK {
		t.Fatalf("update manual node: want 200 got %d: %s", updated.Code, updated.Body.String())
	}
	node, err := store.NodeRepo().FindByID(context.Background(), manualID)
	if err != nil || node.Address != "example.net" || node.Port != 1081 || node.SourceURL != "manual" {
		t.Fatalf("manual node update was not preserved: %+v err=%v", node, err)
	}

	upstream := &domain.Upstream{ID: "subscription-read-only", Name: "subscription", URL: "http://unused", Enabled: true, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	if err := store.UpstreamRepo().Save(context.Background(), upstream); err != nil {
		t.Fatal(err)
	}
	subscriptionNode := &domain.Node{ID: "subscription-node", Name: "sub", Protocol: "socks5", Address: "192.0.2.50", Port: 1080, SourceURL: upstream.URL, Status: domain.NodeActive, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	if err := store.NodeRepo().Save(context.Background(), subscriptionNode); err != nil {
		t.Fatal(err)
	}
	if err := store.NodeRepo().SaveNodeSource(context.Background(), domain.NodeSource{NodeID: subscriptionNode.ID, UpstreamID: upstream.ID, CreatedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	blocked := doRequest(t, srv.Handler(), "PUT", "/api/v1/nodes/"+subscriptionNode.ID, map[string]any{"name": "should-not-change"}, authHeader(token))
	if blocked.Code != http.StatusConflict {
		t.Fatalf("subscription update: want 409 got %d: %s", blocked.Code, blocked.Body.String())
	}
	blockedDelete := doRequest(t, srv.Handler(), "DELETE", "/api/v1/nodes/"+subscriptionNode.ID, nil, authHeader(token))
	if blockedDelete.Code != http.StatusConflict {
		t.Fatalf("subscription delete: want 409 got %d: %s", blockedDelete.Code, blockedDelete.Body.String())
	}
}

// ── upstreams refresh records ─────────────────────────────────────────────────

func TestUpstreamRefreshes(t *testing.T) {
	srv, store := newTestServer(t)
	token := login(t, srv.Handler(), "testpass")
	h := srv.Handler()

	// Create upstream
	rr := doRequest(t, h, "POST", "/api/v1/upstreams", map[string]any{
		"name": "refresh-test", "url": "https://example.com/sub",
	}, authHeader(token))
	var uResp map[string]any
	decodeBody(t, rr, &uResp)
	id := uResp["id"].(string)

	// Seed a refresh record
	now := time.Now().UTC()
	rec := &domain.RefreshRecord{
		ID: fmt.Sprintf("rec-%d", now.UnixNano()), UpstreamID: id,
		Success: true, NodeCount: 5, CreatedAt: now,
	}
	if err := store.UpstreamRepo().SaveRefreshRecord(context.Background(), rec); err != nil {
		t.Fatalf("seed refresh record: %v", err)
	}

	rr2 := doRequest(t, h, "GET", "/api/v1/upstreams/"+id+"/refreshes", nil, authHeader(token))
	if rr2.Code != http.StatusOK {
		t.Fatalf("want 200 got %d: %s", rr2.Code, rr2.Body.String())
	}
	var resp map[string]any
	decodeBody(t, rr2, &resp)
	records := resp["refreshes"].([]any)
	if len(records) < 1 {
		t.Fatalf("want at least 1 refresh record got %d", len(records))
	}
}
