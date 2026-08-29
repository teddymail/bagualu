package httptransport

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/teddymail/bagualu/internal/domain"
)

// GET /api/v1/dashboard/summary
func (s *Server) dashboardSummary(w http.ResponseWriter, r *http.Request) {
	summary := map[string]any{
		"core": s.safeCoreStatus(),
	}
	if s.store != nil {
		nodes, _ := s.store.NodeRepo().FindAll(r.Context(), domainNodeFilter{})
		groups, _ := s.store.GroupRepo().FindAll(r.Context())
		upstreams, _ := s.store.UpstreamRepo().FindAll(r.Context())
		summary["node_count"] = len(nodes)
		summary["group_count"] = len(groups)
		summary["upstream_count"] = len(upstreams)
		statusCounts := map[string]int{}
		protocolCounts := map[string]int{}
		for _, node := range nodes {
			statusCounts[string(node.Status)]++
			protocolCounts[node.Protocol]++
		}
		summary["node_status_counts"] = statusCounts
		summary["protocol_counts"] = protocolCounts
		if jobs, err := s.store.JobRepo().FindActive(r.Context(), 20); err == nil {
			summary["recent_tasks"] = jobs
		}
	}
	if s.coreTraffic != nil {
		summary["traffic"] = s.coreTraffic()
	}
	writeJSON(w, http.StatusOK, summary)
}

// GET /api/v1/system/status
func (s *Server) systemStatus(w http.ResponseWriter, r *http.Request) {
	serviceState := "stopped"
	if status, ok := s.safeCoreStatus().(domain.CoreStatus); ok {
		serviceState = status.State
		if serviceState == "" && status.Available {
			serviceState = "running"
		}
		if serviceState == "" {
			serviceState = "degraded"
		}
	}
	response := map[string]any{
		"service": serviceState,
		"core":    s.safeCoreStatus(),
		"go":      runtime.Version(),
		"os":      runtime.GOOS + "/" + runtime.GOARCH,
	}
	if s.coreInstallStatusFn != nil {
		response["core_install"] = s.coreInstallStatusFn(r.Context())
	}
	writeJSON(w, http.StatusOK, response)
}

// GET /api/v1/system/core/status
func (s *Server) coreStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, coreStatusPayload(s.safeCoreStatus()))
}

func (s *Server) coreInstallStatus(w http.ResponseWriter, r *http.Request) {
	if s.coreInstallStatusFn == nil {
		writeJSON(w, http.StatusOK, domain.CoreInstallStatus{Architecture: runtime.GOOS + "/" + runtime.GOARCH})
		return
	}
	writeJSON(w, http.StatusOK, s.coreInstallStatusFn(r.Context()))
}

func (s *Server) coreInstall(w http.ResponseWriter, r *http.Request) {
	if s.coreInstallFn == nil {
		apiErr(w, http.StatusNotImplemented, "core_installer_unavailable", "mihomo installer is not configured")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Minute)
	defer cancel()
	result, err := s.coreInstallFn(ctx)
	if err != nil {
		apiErr(w, http.StatusBadGateway, "core_install_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "installed", "result": result, "core": s.safeCoreStatus()})
}

func (s *Server) coreInstallUpload(w http.ResponseWriter, r *http.Request) {
	if s.coreInstallUploadFn == nil {
		apiErr(w, http.StatusNotImplemented, "core_installer_unavailable", "mihomo upload installer is not configured")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 256<<20)
	file, header, err := r.FormFile("file")
	if err != nil {
		apiErr(w, http.StatusBadRequest, "bad_request", "mihomo file is required")
		return
	}
	defer file.Close()
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()
	result, err := s.coreInstallUploadFn(ctx, file, filepath.Base(header.Filename))
	if err != nil {
		apiErr(w, http.StatusBadRequest, "core_install_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "installed", "result": result, "core": s.safeCoreStatus()})
}

func coreStatusPayload(value any) any {
	if status, ok := value.(domain.CoreStatus); ok {
		return map[string]any{"available": status.Available, "running": status.Available, "pid": status.PID,
			"version": status.Version, "control": status.Control, "proxy": status.Proxy, "error_code": status.ErrorCode,
			"state": status.State, "auto_restarts": status.AutoRestarts}
	}
	return value
}

// GET /api/v1/system/config
func (s *Server) systemConfigGet(w http.ResponseWriter, r *http.Request) {
	if !s.requireStore(w) {
		return
	}
	cfg, err := s.store.SettingsRepo().GetAll(r.Context())
	if err != nil {
		apiErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	// Never expose sensitive keys
	for key := range cfg {
		lower := strings.ToLower(key)
		if key == settingKeyAdminPasswordHash || strings.Contains(lower, "password") || strings.Contains(lower, "secret") || strings.Contains(lower, "token") || strings.Contains(lower, "private") {
			delete(cfg, key)
		}
	}
	writeJSON(w, http.StatusOK, cfg)
}

// PUT /api/v1/system/config
func (s *Server) systemConfigPut(w http.ResponseWriter, r *http.Request) {
	if !s.requireStore(w) {
		return
	}
	var body map[string]string
	if err := decodeJSON(r, &body); err != nil {
		apiErr(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}
	repo := s.store.SettingsRepo()
	for k, v := range body {
		lower := strings.ToLower(k)
		if k == settingKeyAdminPasswordHash || strings.Contains(lower, "password") || strings.Contains(lower, "secret") || strings.Contains(lower, "token") || strings.Contains(lower, "private") {
			continue // use PUT /auth/password instead
		}
		if err := repo.Set(r.Context(), k, v); err != nil {
			apiErr(w, http.StatusInternalServerError, "internal_error", "failed to save key: "+k)
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// GET /api/v1/system/logs
func (s *Server) systemLogs(w http.ResponseWriter, r *http.Request) {
	logs := make([]map[string]any, 0)
	if s.store != nil {
		jobs, err := s.store.JobRepo().FindAll(r.Context(), domainJobFilter{Limit: 100})
		if err == nil {
			for _, job := range jobs {
				logs = append(logs, map[string]any{
					"time": job.UpdatedAt, "level": jobLogLevel(job.Status),
					"message": "任务 " + string(job.Kind) + " 状态: " + string(job.Status),
					"job_id":  job.ID,
				})
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"logs": logs})
}

func (s *Server) coreDiagnose(w http.ResponseWriter, r *http.Request) {
	core := s.safeCoreStatus()
	status := "ok"
	if value, ok := core.(domain.CoreStatus); ok && !value.Available {
		status = "failed"
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": status, "core": core})
}

func (s *Server) coreCapabilities(w http.ResponseWriter, r *http.Request) {
	available := false
	if value, ok := s.safeCoreStatus().(domain.CoreStatus); ok {
		available = value.Available
	}
	protocols := []string{"http", "socks5", "ss", "ssr", "vmess", "vless", "trojan", "hysteria", "hysteria2", "tuic"}
	matrix := make(map[string]any, len(protocols))
	for _, protocol := range protocols {
		entry := map[string]any{"supported": available, "load": available, "test": available}
		if !available {
			entry["reason"] = domain.ErrCodeCoreUnavailable
		}
		matrix[protocol] = entry
	}
	writeJSON(w, http.StatusOK, map[string]any{"capabilities": map[string]any{
		"clash": available, "mihomo": available, "connectivity": available, "ping": true, "throughput": available,
		"protocols": matrix,
		"status":    map[string]any{"available": available},
	}})
}

func (s *Server) coexistenceCheck(w http.ResponseWriter, r *http.Request) {
	conflicts := make([]map[string]any, 0)
	if _, err := os.Stat("/etc/config/openclash"); err == nil {
		conflicts = append(conflicts, map[string]any{"type": "openclash_present", "severity": "info", "message": "OpenClash configuration detected; Bagualu uses its own ports and files"})
	}
	writeJSON(w, http.StatusOK, map[string]any{"conflicts": conflicts, "safe": true})
}

func (s *Server) systemCoreConfigPut(w http.ResponseWriter, r *http.Request) {
	if !s.requireStore(w) {
		return
	}
	s.systemConfigPut(w, r)
}

func (s *Server) systemCoreReload(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		apiErr(w, http.StatusNotImplemented, "store_unavailable", "storage is not configured")
		return
	}
	now := time.Now().UTC()
	job := &domain.Job{ID: uuid.NewString(), Kind: "core_reload", Status: domain.JobPending, CreatedAt: now, UpdatedAt: now}
	if err := s.store.JobRepo().Save(r.Context(), job); err != nil {
		apiErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	go func() {
		ctx := context.Background()
		_ = s.store.JobRepo().UpdateStatus(ctx, job.ID, domain.JobRunning, 10, "")
		if s.coreReload == nil {
			_ = s.store.JobRepo().UpdateStatus(ctx, job.ID, domain.JobSucceeded, 100, "")
			return
		}
		if err := s.coreReload(ctx); err != nil {
			_ = s.store.JobRepo().UpdateStatus(ctx, job.ID, domain.JobFailed, 100, err.Error())
			return
		}
		_ = s.store.JobRepo().UpdateStatus(ctx, job.ID, domain.JobSucceeded, 100, "")
	}()
	writeJSON(w, http.StatusAccepted, map[string]string{"operation_id": job.ID, "status": "scheduled"})
}

func (s *Server) systemOperation(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		apiErr(w, http.StatusNotImplemented, "store_unavailable", "storage is not configured")
		return
	}
	job, err := s.store.JobRepo().FindByID(r.Context(), r.PathValue("id"))
	if err != nil {
		writeNotFoundOrError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"operation_id": job.ID, "status": job.Status, "phase": operationPhase(job), "progress": job.Progress, "error": job.Error, "updated_at": job.UpdatedAt, "finished_at": job.FinishedAt})
}

func operationPhase(job *domain.Job) string {
	if job.IsTerminal() {
		return "finished"
	}
	if job.Progress <= 0 {
		return "submitted"
	}
	if job.Progress < 100 {
		return "running"
	}
	return "finished"
}

func (s *Server) systemAdminPut(w http.ResponseWriter, r *http.Request) {
	// Password changes remain centralized in the authenticated password endpoint.
	var body struct {
		Username string `json:"username"`
	}
	if err := decodeJSON(r, &body); err != nil {
		apiErr(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}
	if body.Username == "" {
		body.Username = "admin"
	}
	if s.store != nil {
		if err := s.store.SettingsRepo().Set(r.Context(), settingKeyAdminUsername, body.Username); err != nil {
			apiErr(w, http.StatusInternalServerError, "internal_error", err.Error())
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]string{"username": body.Username})
}

// GET /api/v1/runtime/summary
func (s *Server) runtimeSummary(w http.ResponseWriter, r *http.Request) {
	resp := map[string]any{
		"queue_length": 0,
		"running":      false,
		"core":         s.safeCoreStatus(),
	}
	if s.runtimeSnapshot != nil {
		for key, value := range s.runtimeSnapshot() {
			resp[key] = value
		}
	}
	if s.store != nil {
		if jobs, err := s.store.JobRepo().FindActive(r.Context(), 100); err == nil {
			resp["running_jobs"] = len(jobs)
			resp["tasks"] = jobs
		}
	}
	if s.coreTraffic != nil {
		resp["traffic"] = s.coreTraffic()
	}
	if s.coreRuntime != nil {
		for key, value := range s.coreRuntime(r.Context()) {
			resp[key] = value
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

// GET /api/v1/runtime/tasks
func (s *Server) runtimeTasks(w http.ResponseWriter, r *http.Request) {
	if !s.requireStore(w) {
		return
	}
	jobs, err := s.store.JobRepo().FindActive(r.Context(), 100)
	if err != nil {
		apiErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"tasks": nilSafeSlice(jobs)})
}

func (s *Server) runtimeCoreInstances(w http.ResponseWriter, r *http.Request) {
	core := s.safeCoreStatus()
	instance := map[string]any{"id": "bagualu-default", "managed": true, "core": core}
	if s.coreRuntime != nil {
		for key, value := range s.coreRuntime(r.Context()) {
			instance[key] = value
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"instances": []any{instance}})
}

func (s *Server) runtimeNodes(w http.ResponseWriter, r *http.Request) {
	if !s.requireStore(w) {
		return
	}
	jobs, err := s.store.JobRepo().FindActive(r.Context(), 100)
	if err != nil {
		apiErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	seen := map[string]bool{}
	nodes := make([]domain.Node, 0)
	for _, job := range jobs {
		if seen[job.EntityID] || !strings.HasPrefix(job.Kind, "test_") {
			continue
		}
		node, findErr := s.store.NodeRepo().FindByID(r.Context(), job.EntityID)
		if findErr == nil {
			nodes = append(nodes, *node)
			seen[job.EntityID] = true
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"nodes": toNodeResponseList(nodes)})
}

func (s *Server) runtimeTraffic(w http.ResponseWriter, r *http.Request) {
	if s.coreTraffic != nil {
		writeJSON(w, http.StatusOK, s.coreTraffic())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"connections": 0, "bagualu_download_bytes": 0, "bagualu_upload_bytes": 0,
		"wan_download_bytes": 0, "wan_upload_bytes": 0, "download_bytes": 0, "upload_bytes": 0,
	})
}

func jobLogLevel(status domain.JobStatus) string {
	if status == domain.JobFailed {
		return "error"
	}
	if status == domain.JobSucceeded {
		return "info"
	}
	return "debug"
}

// GET /api/v1/reports/nodes
func (s *Server) reportsNodes(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		writeJSON(w, http.StatusOK, map[string]any{"nodes": []any{}})
		return
	}
	since := reportSince(r)
	nodes, err := s.store.NodeRepo().FindAll(r.Context(), domainNodeFilter{})
	if err != nil {
		apiErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	enrichNodesWithScore(r, s, nodes)
	reports, err := s.store.ReportRepo().NodeReports(r.Context(), since, r.URL.Query().Get("node_id"), r.URL.Query().Get("kind"))
	if err != nil {
		apiErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"nodes": toNodeResponseList(nodes), "reports": reports, "since": since})
}

// GET /api/v1/reports/summary
func (s *Server) reportsSummary(w http.ResponseWriter, r *http.Request) {
	summary := map[string]any{"node_count": 0, "measurement_count": 0}
	if s.store != nil {
		nodes, _ := s.store.NodeRepo().FindAll(r.Context(), domainNodeFilter{})
		summary["node_count"] = len(nodes)
		if report, err := s.store.ReportRepo().Summary(r.Context(), reportSince(r)); err == nil {
			for key, value := range report {
				summary[key] = value
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"summary": summary})
}

func (s *Server) reportsTraffic(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		writeJSON(w, http.StatusOK, map[string]any{"entries": []any{}})
		return
	}
	reports, err := s.store.ReportRepo().NodeReports(r.Context(), reportSince(r), r.URL.Query().Get("node_id"), "throughput")
	if err != nil {
		apiErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"entries": reports, "since": reportSince(r)})
}

func (s *Server) reportsExport(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		apiErr(w, http.StatusNotImplemented, "store_unavailable", "storage is not configured")
		return
	}
	since := reportSince(r)
	nodes, err := s.store.ReportRepo().NodeReports(r.Context(), since, r.URL.Query().Get("node_id"), r.URL.Query().Get("kind"))
	if err != nil {
		apiErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	w.Header().Set("Content-Disposition", `attachment; filename="bagualu-report.json"`)
	writeJSON(w, http.StatusOK, map[string]any{"since": since, "nodes": nodes})
}

func reportSince(r *http.Request) time.Time {
	if value := r.URL.Query().Get("since"); value != "" {
		if parsed, err := time.Parse(time.RFC3339, value); err == nil {
			return parsed.UTC()
		}
	}
	if days, err := strconv.Atoi(r.URL.Query().Get("days")); err == nil && days > 0 && days <= 3650 {
		return time.Now().UTC().Add(-time.Duration(days) * 24 * time.Hour)
	}
	return time.Now().UTC().Add(-7 * 24 * time.Hour)
}

// safeCoreStatus calls CoreStatus if available, or returns a zero-value JSON object.
func (s *Server) safeCoreStatus() any {
	if s.CoreStatus != nil {
		return s.CoreStatus()
	}
	return map[string]any{"available": false}
}
