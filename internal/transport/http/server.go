// Package httptransport implements the Bagualu HTTP management and resource API.
// Section references: SDD §9 (management API) and §10 (external resource API).
package httptransport

import (
	"context"
	"embed"
	"encoding/json"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/teddymail/bagualu/internal/domain"
	"github.com/teddymail/bagualu/internal/infrastructure/persistence"
)

//go:embed static/*
var adminAssets embed.FS

// Config holds all dependencies for the full-featured server.
type Config struct {
	// CoreStatus returns the current Mihomo core status.
	CoreStatus  func() domain.CoreStatus
	CoreTraffic func() map[string]any
	CoreRuntime func(context.Context) map[string]any
	// Store provides all repository factories. May be nil for minimal/legacy use.
	Store *persistence.Store
	// AdminPassword is the plain-text password used to bootstrap the admin
	// account when no password has been set in the database yet.
	// Pass an empty string to skip bootstrapping.
	AdminPassword string
	// TestSubmit enqueues a node test on the shared single-threaded runner.
	TestSubmit func(context.Context, string, domain.TestKind) (string, error)
	// RefreshSubmit starts an asynchronous upstream refresh and returns its job ID.
	RefreshSubmit    func(context.Context, string) (string, error)
	TestCancel       func(context.Context, string) (bool, error)
	ScoreRecalculate func(context.Context, string) (string, error)
	RuntimeSnapshot  func() map[string]any
	CoreReload       func(context.Context) error
	ScorePolicyGet   func() domain.ScorePolicy
	ScorePolicySet   func(domain.ScorePolicy) error
}

// Server is the Bagualu HTTP server. It exposes management and resource APIs.
// All dependencies are optional: when Store is nil, repository-backed handlers
// respond with 501 Not Implemented so that legacy single-function usage keeps working.
type Server struct {
	CoreStatus       func() domain.CoreStatus
	coreTraffic      func() map[string]any
	coreRuntime      func(context.Context) map[string]any
	store            *persistence.Store
	sessions         *sessionStore
	testSubmit       func(context.Context, string, domain.TestKind) (string, error)
	refreshSubmit    func(context.Context, string) (string, error)
	testCancel       func(context.Context, string) (bool, error)
	scoreRecalculate func(context.Context, string) (string, error)
	runtimeSnapshot  func() map[string]any
	coreReload       func(context.Context) error
	scorePolicyGet   func() domain.ScorePolicy
	scorePolicySet   func(domain.ScorePolicy) error
}

// NewServer constructs a minimal server from only a CoreStatus function.
// This overload preserves backward compatibility with the existing cmd/ initialiser.
func NewServer(status func() domain.CoreStatus) *Server {
	return &Server{CoreStatus: status, sessions: newSessionStore()}
}

// NewServerWithConfig constructs a fully-featured server from a Config.
// When cfg.Store is non-nil all CRUD and resource endpoints are wired.
// Call Bootstrap after construction to initialise the default admin password.
func NewServerWithConfig(cfg Config) *Server {
	s := &Server{
		CoreStatus:       cfg.CoreStatus,
		coreTraffic:      cfg.CoreTraffic,
		coreRuntime:      cfg.CoreRuntime,
		store:            cfg.Store,
		sessions:         newSessionStore(),
		testSubmit:       cfg.TestSubmit,
		refreshSubmit:    cfg.RefreshSubmit,
		testCancel:       cfg.TestCancel,
		scoreRecalculate: cfg.ScoreRecalculate,
		runtimeSnapshot:  cfg.RuntimeSnapshot,
		coreReload:       cfg.CoreReload,
		scorePolicyGet:   cfg.ScorePolicyGet,
		scorePolicySet:   cfg.ScorePolicySet,
	}
	if cfg.Store != nil && cfg.AdminPassword != "" {
		ctx := noopCtx{}
		existing, _ := cfg.Store.SettingsRepo().Get(ctx, settingKeyAdminPasswordHash)
		if existing == "" {
			hash := hashPassword(cfg.AdminPassword)
			_ = cfg.Store.SettingsRepo().Set(ctx, settingKeyAdminPasswordHash, hash)
		}
	}
	return s
}

// Handler returns the root http.Handler for the server.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	// ── public ──────────────────────────────────────────────────────────────
	mux.HandleFunc("GET /api/v1/health", s.health)
	mux.HandleFunc("POST /api/v1/auth/login", s.authLogin)
	mux.HandleFunc("/", s.adminApp)

	// ── management (admin Bearer token required) ─────────────────────────────
	mux.HandleFunc("POST /api/v1/auth/logout", s.adminRequired(s.authLogout))
	mux.HandleFunc("GET /api/v1/auth/me", s.adminRequired(s.authMe))
	mux.HandleFunc("PUT /api/v1/auth/password", s.adminRequired(s.authChangePassword))

	mux.HandleFunc("GET /api/v1/dashboard/summary", s.adminRequired(s.dashboardSummary))

	mux.HandleFunc("GET /api/v1/system/status", s.adminRequired(s.systemStatus))
	mux.HandleFunc("GET /api/v1/system/core/status", s.adminRequired(s.coreStatus))
	mux.HandleFunc("POST /api/v1/system/core/diagnose", s.adminRequired(s.coreDiagnose))
	mux.HandleFunc("GET /api/v1/system/core/capabilities", s.adminRequired(s.coreCapabilities))
	mux.HandleFunc("POST /api/v1/system/coexistence/check", s.adminRequired(s.coexistenceCheck))
	mux.HandleFunc("PUT /api/v1/system/core/config", s.adminRequired(s.systemCoreConfigPut))
	mux.HandleFunc("POST /api/v1/system/core/reload", s.adminRequired(s.systemCoreReload))
	mux.HandleFunc("GET /api/v1/system/config", s.adminRequired(s.systemConfigGet))
	mux.HandleFunc("PUT /api/v1/system/config", s.adminRequired(s.systemConfigPut))
	mux.HandleFunc("GET /api/v1/system/operations/{id}", s.adminRequired(s.systemOperation))
	mux.HandleFunc("GET /api/v1/system/logs", s.adminRequired(s.systemLogs))
	mux.HandleFunc("GET /api/v1/system/logs/stream", s.adminRequired(s.systemLogs))
	mux.HandleFunc("GET /api/v1/system/core/logs", s.adminRequired(s.systemLogs))
	mux.HandleFunc("PUT /api/v1/system/admin", s.adminRequired(s.systemAdminPut))

	mux.HandleFunc("GET /api/v1/runtime/summary", s.adminRequired(s.runtimeSummary))
	mux.HandleFunc("GET /api/v1/runtime/tasks", s.adminRequired(s.runtimeTasks))
	mux.HandleFunc("GET /api/v1/runtime/core-instances", s.adminRequired(s.runtimeCoreInstances))
	mux.HandleFunc("GET /api/v1/runtime/nodes", s.adminRequired(s.runtimeNodes))
	mux.HandleFunc("GET /api/v1/runtime/traffic", s.adminRequired(s.runtimeTraffic))

	mux.HandleFunc("GET /api/v1/reports/traffic", s.adminRequired(s.reportsTraffic))
	mux.HandleFunc("GET /api/v1/reports/nodes", s.adminRequired(s.reportsNodes))
	mux.HandleFunc("GET /api/v1/reports/summary", s.adminRequired(s.reportsSummary))
	mux.HandleFunc("GET /api/v1/reports/export", s.adminRequired(s.reportsExport))

	// upstreams
	mux.HandleFunc("GET /api/v1/upstreams", s.adminRequired(s.listUpstreams))
	mux.HandleFunc("POST /api/v1/upstreams", s.adminRequired(s.createUpstream))
	mux.HandleFunc("GET /api/v1/upstreams/{id}", s.adminRequired(s.getUpstream))
	mux.HandleFunc("PUT /api/v1/upstreams/{id}", s.adminRequired(s.updateUpstream))
	mux.HandleFunc("DELETE /api/v1/upstreams/{id}", s.adminRequired(s.deleteUpstream))
	mux.HandleFunc("POST /api/v1/upstreams/{id}/refresh", s.adminRequired(s.refreshUpstream))
	mux.HandleFunc("POST /api/v1/upstreams/{id}/tests/throughput", s.adminRequired(s.testUpstream))
	mux.HandleFunc("GET /api/v1/upstreams/{id}/refreshes", s.adminRequired(s.listUpstreamRefreshes))

	// nodes
	mux.HandleFunc("GET /api/v1/nodes", s.adminRequired(s.listNodes))
	mux.HandleFunc("POST /api/v1/nodes", s.adminRequired(s.createNode))
	mux.HandleFunc("GET /api/v1/nodes/{id}", s.adminRequired(s.getNode))
	mux.HandleFunc("PUT /api/v1/nodes/{id}", s.adminRequired(s.updateNode))
	mux.HandleFunc("POST /api/v1/nodes/{id}/enable", s.adminRequired(s.enableNode))
	mux.HandleFunc("POST /api/v1/nodes/{id}/disable", s.adminRequired(s.disableNode))
	mux.HandleFunc("DELETE /api/v1/nodes/{id}", s.adminRequired(s.deleteNode))
	mux.HandleFunc("GET /api/v1/nodes/{id}/measurements", s.adminRequired(s.nodeMeasurements))
	mux.HandleFunc("GET /api/v1/nodes/{id}/tests/capability", s.adminRequired(s.nodeCapability))
	mux.HandleFunc("POST /api/v1/nodes/{id}/tests/connectivity", s.adminRequired(s.createNodeTest))
	mux.HandleFunc("POST /api/v1/nodes/{id}/tests/ping", s.adminRequired(s.createNodeTest))
	mux.HandleFunc("POST /api/v1/nodes/{id}/tests/throughput", s.adminRequired(s.createNodeTest))
	mux.HandleFunc("GET /api/v1/nodes/{id}/score", s.adminRequired(s.nodeScore))
	mux.HandleFunc("GET /api/v1/nodes/{id}/score/events", s.adminRequired(s.nodeScoreEvents))
	mux.HandleFunc("POST /api/v1/nodes/{id}/score/recalculate", s.adminRequired(s.nodeScoreRecalculate))

	// groups
	mux.HandleFunc("GET /api/v1/groups", s.adminRequired(s.listGroups))
	mux.HandleFunc("POST /api/v1/groups", s.adminRequired(s.createGroup))
	mux.HandleFunc("GET /api/v1/groups/{id}", s.adminRequired(s.getGroup))
	mux.HandleFunc("PUT /api/v1/groups/{id}", s.adminRequired(s.updateGroup))
	mux.HandleFunc("DELETE /api/v1/groups/{id}", s.adminRequired(s.deleteGroup))
	mux.HandleFunc("GET /api/v1/groups/{id}/nodes", s.adminRequired(s.getGroupNodes))
	mux.HandleFunc("PUT /api/v1/groups/{id}/nodes", s.adminRequired(s.setGroupNodes))
	mux.HandleFunc("POST /api/v1/groups/{id}/tests", s.adminRequired(s.createGroupTests))
	mux.HandleFunc("GET /api/v1/groups/{id}/test-policy", s.adminRequired(s.getGroupTestPolicy))
	mux.HandleFunc("PUT /api/v1/groups/{id}/test-policy", s.adminRequired(s.putGroupTestPolicy))

	// test policy (global)
	mux.HandleFunc("GET /api/v1/test-policy", s.adminRequired(s.getTestPolicy))
	mux.HandleFunc("PUT /api/v1/test-policy", s.adminRequired(s.putTestPolicy))

	// jobs
	mux.HandleFunc("POST /api/v1/jobs/clear", s.adminRequired(s.clearJobs))
	mux.HandleFunc("GET /api/v1/jobs", s.adminRequired(s.listJobs))
	mux.HandleFunc("GET /api/v1/jobs/{id}", s.adminRequired(s.getJob))
	mux.HandleFunc("POST /api/v1/jobs/{id}/cancel", s.adminRequired(s.cancelJob))

	// API keys
	mux.HandleFunc("GET /api/v1/api-keys", s.adminRequired(s.listAPIKeys))
	mux.HandleFunc("POST /api/v1/api-keys", s.adminRequired(s.createAPIKey))
	mux.HandleFunc("PUT /api/v1/api-keys/{id}", s.adminRequired(s.updateAPIKey))
	mux.HandleFunc("POST /api/v1/api-keys/{id}/rotate", s.adminRequired(s.rotateAPIKey))
	mux.HandleFunc("POST /api/v1/api-keys/{id}/revoke", s.adminRequired(s.revokeAPIKey))

	// subscription links
	mux.HandleFunc("GET /api/v1/subscription-links", s.adminRequired(s.listSubscriptionLinks))
	mux.HandleFunc("POST /api/v1/subscription-links", s.adminRequired(s.createSubscriptionLink))
	mux.HandleFunc("GET /api/v1/subscription-links/{id}", s.adminRequired(s.getSubscriptionLink))
	mux.HandleFunc("PUT /api/v1/subscription-links/{id}", s.adminRequired(s.updateSubscriptionLink))
	mux.HandleFunc("DELETE /api/v1/subscription-links/{id}", s.adminRequired(s.deleteSubscriptionLink))
	mux.HandleFunc("POST /api/v1/subscription-links/{id}/rotate", s.adminRequired(s.rotateSubscriptionLink))
	mux.HandleFunc("GET /api/v1/subscription-links/{id}/preview", s.adminRequired(s.previewSubscriptionLink))
	mux.HandleFunc("POST /api/v1/subscription-links/{id}/detect-preview", s.adminRequired(s.detectPreviewSubscriptionLink))
	mux.HandleFunc("GET /api/v1/client-detection-rules", s.adminRequired(s.getClientDetectionRules))
	mux.HandleFunc("PUT /api/v1/client-detection-rules", s.adminRequired(s.putClientDetectionRules))

	// score policy
	mux.HandleFunc("GET /api/v1/score-policy", s.adminRequired(s.getScorePolicy))
	mux.HandleFunc("PUT /api/v1/score-policy", s.adminRequired(s.putScorePolicy))

	// regions
	mux.HandleFunc("GET /api/v1/regions", s.adminRequired(s.listRegions))
	mux.HandleFunc("PUT /api/v1/regions/{code}", s.adminRequired(s.updateRegion))

	// ── resource API (API key Bearer token required) ──────────────────────────
	mux.HandleFunc("GET /api/v1/resources", s.apikeyRequired(s.resourcesDefault))
	mux.HandleFunc("GET /api/v1/resources/{group}", s.apikeyRequired(s.resourcesGroup))
	mux.HandleFunc("GET /api/v1/resources/{group}/nodes", s.apikeyRequired(s.resourcesGroupNodes))
	mux.HandleFunc("GET /api/v1/resources/{group}/subscription", s.apikeyRequired(s.resourcesGroupSubscription))
	mux.HandleFunc("GET /api/v1/resources/{group}/stats", s.apikeyRequired(s.resourcesGroupStats))

	// ── public subscription token endpoint ───────────────────────────────────
	mux.HandleFunc("GET /api/v1/subscribe/{token}", s.subscribeByToken)

	// catch-all for unregistered /api/v1/ paths
	mux.HandleFunc("/api/v1/", s.notFound)

	return withCommonHeaders(mux)
}

func (s *Server) adminApp(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
	if name == "" || strings.HasPrefix(name, "api/") {
		name = "index.html"
	}
	file, err := adminAssets.ReadFile("static/" + name)
	if err != nil {
		file, err = adminAssets.ReadFile("static/index.html")
		if err != nil {
			http.Error(w, "admin frontend unavailable", http.StatusServiceUnavailable)
			return
		}
	}
	if strings.HasSuffix(name, ".html") || name == "index.html" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
	}
	http.ServeContent(w, r, name, time.Time{}, strings.NewReader(string(file)))
}

// withCommonHeaders adds security and content headers to every response.
func withCommonHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		next.ServeHTTP(w, r)
	})
}

// writeJSON serialises value as JSON and writes it with the given status code.
func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

// writeText writes a plain-text body with the given Content-Type and status.
func writeText(w http.ResponseWriter, status int, contentType string, body []byte) {
	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

// apiErr writes a structured JSON error body compatible with SDD §9.
func apiErr(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]string{"error_code": code, "message": message})
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) notFound(w http.ResponseWriter, r *http.Request) {
	apiErr(w, http.StatusNotFound, "not_found", "endpoint not found")
}

// requireStore checks that the store is available; returns false and writes a
// 501 response if it is nil so that handlers can early-return.
func (s *Server) requireStore(w http.ResponseWriter) bool {
	if s.store == nil {
		apiErr(w, http.StatusNotImplemented, "not_implemented", "store not configured")
		return false
	}
	return true
}

// noopCtx is a minimal context that never cancels, used for bootstrap calls
// that happen before an HTTP request is in flight.
type noopCtx struct{}

func (noopCtx) Deadline() (deadline time.Time, ok bool) { return time.Time{}, false }
func (noopCtx) Done() <-chan struct{}                   { return nil }
func (noopCtx) Err() error                              { return nil }
func (noopCtx) Value(key any) any                       { return nil }
