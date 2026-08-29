package httptransport

import (
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/teddymail/bagualu/internal/domain"
)

// GET /api/v1/upstreams
func (s *Server) listUpstreams(w http.ResponseWriter, r *http.Request) {
	if !s.requireStore(w) {
		return
	}
	list, err := s.store.UpstreamRepo().FindAll(r.Context())
	if err != nil {
		apiErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	resp := make([]upstreamResponse, 0, len(list))
	for i := range list {
		item := toUpstreamResponse(&list[i])
		if records, refreshErr := s.store.UpstreamRepo().FindRefreshRecords(r.Context(), list[i].ID, 1); refreshErr == nil && len(records) > 0 {
			latest := toRefreshRecordResponse(records[0])
			item.LastRefresh = &latest
		}
		resp = append(resp, item)
	}
	writeJSON(w, http.StatusOK, map[string]any{"upstreams": resp})
}

// POST /api/v1/upstreams
func (s *Server) createUpstream(w http.ResponseWriter, r *http.Request) {
	if !s.requireStore(w) {
		return
	}
	var body struct {
		Name            string `json:"name"`
		URL             string `json:"url"`
		Format          string `json:"format"`
		RefreshInterval int    `json:"refresh_interval_seconds"`
		Enabled         bool   `json:"enabled"`
		Notes           string `json:"notes"`
	}
	if err := decodeJSON(r, &body); err != nil {
		apiErr(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}
	if body.Name == "" || body.URL == "" {
		apiErr(w, http.StatusBadRequest, "bad_request", "name and url are required")
		return
	}
	now := time.Now().UTC()
	u := &domain.Upstream{
		ID:              uuid.NewString(),
		Name:            body.Name,
		URL:             body.URL,
		Format:          domain.UpstreamFormat(body.Format),
		RefreshInterval: time.Duration(body.RefreshInterval) * time.Second,
		Enabled:         body.Enabled,
		Notes:           body.Notes,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if err := s.store.UpstreamRepo().Save(r.Context(), u); err != nil {
		apiErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, toUpstreamResponse(u))
}

// GET /api/v1/upstreams/{id}
func (s *Server) getUpstream(w http.ResponseWriter, r *http.Request) {
	if !s.requireStore(w) {
		return
	}
	id := r.PathValue("id")
	u, err := s.store.UpstreamRepo().FindByID(r.Context(), id)
	if err != nil {
		writeNotFoundOrError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toUpstreamResponse(u))
}

// PUT /api/v1/upstreams/{id}
func (s *Server) updateUpstream(w http.ResponseWriter, r *http.Request) {
	if !s.requireStore(w) {
		return
	}
	id := r.PathValue("id")
	u, err := s.store.UpstreamRepo().FindByID(r.Context(), id)
	if err != nil {
		writeNotFoundOrError(w, err)
		return
	}
	var body struct {
		Name            *string `json:"name"`
		URL             *string `json:"url"`
		Format          *string `json:"format"`
		RefreshInterval *int    `json:"refresh_interval_seconds"`
		Enabled         *bool   `json:"enabled"`
		Notes           *string `json:"notes"`
	}
	if err := decodeJSON(r, &body); err != nil {
		apiErr(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}
	if body.Name != nil {
		u.Name = *body.Name
	}
	if body.URL != nil {
		u.URL = *body.URL
	}
	if body.Format != nil {
		u.Format = domain.UpstreamFormat(*body.Format)
	}
	if body.RefreshInterval != nil {
		u.RefreshInterval = time.Duration(*body.RefreshInterval) * time.Second
	}
	if body.Enabled != nil {
		u.Enabled = *body.Enabled
	}
	if body.Notes != nil {
		u.Notes = *body.Notes
	}
	u.UpdatedAt = time.Now().UTC()
	if err := s.store.UpstreamRepo().Save(r.Context(), u); err != nil {
		apiErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, toUpstreamResponse(u))
}

// DELETE /api/v1/upstreams/{id}
func (s *Server) deleteUpstream(w http.ResponseWriter, r *http.Request) {
	if !s.requireStore(w) {
		return
	}
	id := r.PathValue("id")
	if err := s.store.UpstreamRepo().Delete(r.Context(), id); err != nil {
		writeNotFoundOrError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// POST /api/v1/upstreams/{id}/refresh
func (s *Server) refreshUpstream(w http.ResponseWriter, r *http.Request) {
	if !s.requireStore(w) {
		return
	}

	id := r.PathValue("id")
	_, err := s.store.UpstreamRepo().FindByID(r.Context(), id)
	if err != nil {
		writeNotFoundOrError(w, err)
		return
	}
	if s.refreshSubmit != nil {
		jobID, submitErr := s.refreshSubmit(r.Context(), id)
		if submitErr != nil {
			apiErr(w, http.StatusServiceUnavailable, "refresh_unavailable", submitErr.Error())
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]string{"job_id": jobID})
		return
	}
	// Create a refresh job
	now := time.Now().UTC()
	job := &domain.Job{
		ID:        uuid.NewString(),
		Kind:      "refresh_upstream",
		Status:    domain.JobPending,
		EntityID:  id,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := s.store.JobRepo().Save(r.Context(), job); err != nil {
		apiErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"job_id": job.ID})
}

// POST /api/v1/upstreams/{id}/tests/throughput
func (s *Server) testUpstream(w http.ResponseWriter, r *http.Request) {
	if !s.requireStore(w) {
		return
	}
	upstream, err := s.store.UpstreamRepo().FindByID(r.Context(), r.PathValue("id"))
	if err != nil {
		writeNotFoundOrError(w, err)
		return
	}
	nodes, err := s.store.NodeRepo().FindAll(r.Context(), domain.NodeFilter{})
	if err != nil {
		apiErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	jobIDs := make([]string, 0)
	skipped := make([]map[string]string, 0)
	for i := range nodes {
		if upstream.URL != "" && !strings.HasPrefix(nodes[i].SourceURL, upstream.URL) && !strings.HasPrefix(upstream.URL, nodes[i].SourceURL) {
			continue
		}
		if s.testSubmit == nil {
			continue
		}
		jobID, err := s.testSubmit(r.Context(), nodes[i].ID, domain.TestThroughput)
		if err != nil {
			if isTestQueueFull(err) {
				skipped = append(skipped, map[string]string{"node_id": nodes[i].ID, "reason": "test_queue_full"})
				break
			}
			apiErr(w, http.StatusServiceUnavailable, "core_unavailable", err.Error())
			return
		}
		jobIDs = append(jobIDs, jobID)
	}
	if len(jobIDs) == 0 && len(skipped) > 0 {
		apiErr(w, http.StatusTooManyRequests, "test_queue_full", "test queue is full; retry later")
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{
		"job_ids":       jobIDs,
		"node_count":    len(jobIDs),
		"queued_count":  len(jobIDs),
		"skipped_count": len(skipped),
		"skipped":       skipped,
	})
}

// GET /api/v1/upstreams/{id}/refreshes
func (s *Server) listUpstreamRefreshes(w http.ResponseWriter, r *http.Request) {
	if !s.requireStore(w) {
		return
	}
	id := r.PathValue("id")
	records, err := s.store.UpstreamRepo().FindRefreshRecords(r.Context(), id, 50)
	if err != nil {
		apiErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	resp := make([]refreshRecordResponse, 0, len(records))
	for _, rec := range records {
		resp = append(resp, toRefreshRecordResponse(rec))
	}
	writeJSON(w, http.StatusOK, map[string]any{"refreshes": resp})
}

func nilSafeUpstreams(list []domain.Upstream) any {
	if list == nil {
		return []domain.Upstream{}
	}
	return list
}
