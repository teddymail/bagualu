package httptransport

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/teddymail/bagualu/internal/domain"
)

// GET /api/v1/groups
func (s *Server) listGroups(w http.ResponseWriter, r *http.Request) {
	if !s.requireStore(w) {
		return
	}
	groups, err := s.store.GroupRepo().FindAll(r.Context())
	if err != nil {
		apiErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	resp := make([]groupResponse, 0, len(groups))
	for i := range groups {
		resp = append(resp, toGroupResponse(&groups[i]))
	}
	writeJSON(w, http.StatusOK, map[string]any{"groups": resp})
}

// POST /api/v1/groups
func (s *Server) createGroup(w http.ResponseWriter, r *http.Request) {
	if !s.requireStore(w) {
		return
	}
	var body struct {
		Name             string  `json:"name"`
		Description      string  `json:"description"`
		MinScore         float64 `json:"min_score"`
		OnePerEndpointIP bool    `json:"one_per_endpoint_ip"`
	}
	if err := decodeJSON(r, &body); err != nil {
		apiErr(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}
	if body.Name == "" {
		apiErr(w, http.StatusBadRequest, "bad_request", "name is required")
		return
	}
	now := time.Now().UTC()
	g := &domain.Group{
		ID:               uuid.NewString(),
		Name:             body.Name,
		Description:      body.Description,
		MinScore:         body.MinScore,
		OnePerEndpointIP: body.OnePerEndpointIP,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	if err := s.store.GroupRepo().Save(r.Context(), g); err != nil {
		apiErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, toGroupResponse(g))
}

// GET /api/v1/groups/{id}
func (s *Server) getGroup(w http.ResponseWriter, r *http.Request) {
	if !s.requireStore(w) {
		return
	}
	id := r.PathValue("id")
	g, err := s.store.GroupRepo().FindByID(r.Context(), id)
	if err != nil {
		writeNotFoundOrError(w, err)
		return
	}
	nodeIDs, _ := s.store.GroupRepo().FindNodeIDs(r.Context(), id)
	writeJSON(w, http.StatusOK, map[string]any{"group": toGroupResponse(g), "node_count": len(nodeIDs)})
}

// PUT /api/v1/groups/{id}
func (s *Server) updateGroup(w http.ResponseWriter, r *http.Request) {
	if !s.requireStore(w) {
		return
	}
	id := r.PathValue("id")
	g, err := s.store.GroupRepo().FindByID(r.Context(), id)
	if err != nil {
		writeNotFoundOrError(w, err)
		return
	}
	var body struct {
		Name             *string  `json:"name"`
		Description      *string  `json:"description"`
		MinScore         *float64 `json:"min_score"`
		OnePerEndpointIP *bool    `json:"one_per_endpoint_ip"`
	}
	if err := decodeJSON(r, &body); err != nil {
		apiErr(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}
	if body.Name != nil {
		g.Name = *body.Name
	}
	if body.Description != nil {
		g.Description = *body.Description
	}
	if body.MinScore != nil {
		g.MinScore = *body.MinScore
	}
	if body.OnePerEndpointIP != nil {
		g.OnePerEndpointIP = *body.OnePerEndpointIP
	}
	g.UpdatedAt = time.Now().UTC()
	if err := s.store.GroupRepo().Save(r.Context(), g); err != nil {
		apiErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, toGroupResponse(g))
}

// DELETE /api/v1/groups/{id}
func (s *Server) deleteGroup(w http.ResponseWriter, r *http.Request) {
	if !s.requireStore(w) {
		return
	}
	id := r.PathValue("id")
	if err := s.store.GroupRepo().Delete(r.Context(), id); err != nil {
		writeNotFoundOrError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// GET /api/v1/groups/{id}/nodes
func (s *Server) getGroupNodes(w http.ResponseWriter, r *http.Request) {
	if !s.requireStore(w) {
		return
	}
	id := r.PathValue("id")
	nodeIDs, err := s.store.GroupRepo().FindNodeIDs(r.Context(), id)
	if err != nil {
		apiErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	nodes := make([]domain.Node, 0)
	for _, nid := range nodeIDs {
		n, err := s.store.NodeRepo().FindByID(r.Context(), nid)
		if err != nil {
			continue
		}
		snapshot, _ := s.store.ScoreSnapshotRepo().FindLatestByNodeID(r.Context(), nid)
		if snapshot != nil {
			score := snapshot.ToScore()
			n.Score = &score
		}
		nodes = append(nodes, *n)
	}
	writeJSON(w, http.StatusOK, map[string]any{"nodes": toNodeResponseList(nodes)})
}

// PUT /api/v1/groups/{id}/nodes
func (s *Server) setGroupNodes(w http.ResponseWriter, r *http.Request) {
	if !s.requireStore(w) {
		return
	}
	id := r.PathValue("id")
	var body struct {
		NodeIDs []string `json:"node_ids"`
	}
	if err := decodeJSON(r, &body); err != nil {
		apiErr(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}
	if err := s.store.GroupRepo().SetNodes(r.Context(), id, body.NodeIDs); err != nil {
		apiErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// GET /api/v1/groups/{id}/test-policy
func (s *Server) getGroupTestPolicy(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if s.store != nil {
		if _, err := s.store.GroupRepo().FindByID(r.Context(), id); err != nil {
			writeNotFoundOrError(w, err)
			return
		}
	}
	policy := defaultTestPolicy()
	if s.store != nil {
		if raw, err := s.store.SettingsRepo().Get(r.Context(), "test_policy_group:"+id); err == nil && raw != "" {
			_ = json.Unmarshal([]byte(raw), &policy)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"group_id": id,
		"policy":   policy,
	})
}

// PUT /api/v1/groups/{id}/test-policy
func (s *Server) putGroupTestPolicy(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if s.store != nil {
		if _, err := s.store.GroupRepo().FindByID(r.Context(), id); err != nil {
			writeNotFoundOrError(w, err)
			return
		}
	}
	var body map[string]any
	if err := decodeJSON(r, &body); err != nil {
		apiErr(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}
	if err := validateTestPolicy(body); err != nil {
		apiErr(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	if s.store != nil {
		encoded, err := json.Marshal(body)
		if err != nil || s.store.SettingsRepo().Set(r.Context(), "test_policy_group:"+id, string(encoded)) != nil {
			apiErr(w, http.StatusInternalServerError, "internal_error", "failed to save group test policy")
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"group_id": id, "policy": body})
}

// GET /api/v1/test-policy
func (s *Server) getTestPolicy(w http.ResponseWriter, r *http.Request) {
	policy := defaultTestPolicy()
	if s.store != nil {
		if raw, err := s.store.SettingsRepo().Get(r.Context(), "test_policy"); err == nil && raw != "" {
			_ = json.Unmarshal([]byte(raw), &policy)
		}
	}
	policy["max_concurrent"] = 1
	writeJSON(w, http.StatusOK, policy)
}

// PUT /api/v1/test-policy
func (s *Server) putTestPolicy(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	if err := decodeJSON(r, &body); err != nil {
		apiErr(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}
	if err := validateTestPolicy(body); err != nil {
		apiErr(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	if s.store != nil {
		encoded, err := json.Marshal(body)
		if err != nil || s.store.SettingsRepo().Set(r.Context(), "test_policy", string(encoded)) != nil {
			apiErr(w, http.StatusInternalServerError, "internal_error", "failed to save test policy")
			return
		}
	}
	writeJSON(w, http.StatusOK, body)
}

func defaultTestPolicy() map[string]any {
	return map[string]any{
		"throughput_enabled":   true,
		"connectivity_url":     "http://www.gstatic.com/generate_204",
		"connectivity_timeout": 5,
		"throughput_url":       "https://speed.cloudflare.com/__down?bytes=1048576",
		"throughput_bytes":     1048576,
		"interval_seconds":     60,
		"max_concurrent":       1,
		"retry_count":          2,
		"allowed_windows":      []string{"02:00-06:00"},
		"wan_download_bps":     0,
		"wan_upload_bps":       0,
		"load_threshold":       0.1,
		"speed_sources":        []string{"https://speed.cloudflare.com/__down?bytes=1048576"},
	}
}

func validateTestPolicy(policy map[string]any) error {
	if value, ok := policy["max_concurrent"]; ok {
		if number, valid := numericPolicyValue(value); !valid || number != 1 {
			return fmt.Errorf("max_concurrent is fixed at 1")
		}
	}
	for _, key := range []string{"connectivity_timeout", "throughput_bytes", "interval_seconds", "retry_count"} {
		if value, ok := policy[key]; ok {
			number, valid := numericPolicyValue(value)
			if !valid || number < 0 || (key == "throughput_bytes" && (number < 1024 || number > 100*1024*1024)) || (key == "retry_count" && number > 5) {
				return fmt.Errorf("invalid %s", key)
			}
		}
	}
	if value, ok := policy["load_threshold"]; ok {
		number, valid := numericPolicyValue(value)
		if !valid || number <= 0 || number > 1 {
			return fmt.Errorf("invalid load_threshold")
		}
	}
	return nil
}

func numericPolicyValue(value any) (float64, bool) {
	switch number := value.(type) {
	case float64:
		return number, !math.IsNaN(number) && !math.IsInf(number, 0)
	case int:
		return float64(number), true
	default:
		return 0, false
	}
}

// POST /api/v1/groups/{id}/tests
func (s *Server) createGroupTests(w http.ResponseWriter, r *http.Request) {
	if !s.requireStore(w) {
		return
	}
	id := r.PathValue("id")
	now := time.Now().UTC()
	job := &domain.Job{
		ID:        uuid.NewString(),
		Kind:      "test_group",
		Status:    domain.JobPending,
		EntityID:  id,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := s.store.JobRepo().Save(r.Context(), job); err != nil {
		apiErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	jobIDs := make([]string, 0)
	if s.testSubmit != nil {
		nodeIDs, err := s.store.GroupRepo().FindNodeIDs(r.Context(), id)
		if err != nil {
			apiErr(w, http.StatusInternalServerError, "internal_error", err.Error())
			return
		}
		for _, nodeID := range nodeIDs {
			testJobID, submitErr := s.testSubmit(r.Context(), nodeID, domain.TestConnectivity)
			if submitErr != nil {
				continue
			}
			jobIDs = append(jobIDs, testJobID)
		}
		if len(jobIDs) == 0 {
			_ = s.store.JobRepo().UpdateStatus(r.Context(), job.ID, domain.JobSucceeded, 100, "")
		} else {
			_ = s.store.JobRepo().UpdateStatus(r.Context(), job.ID, domain.JobRunning, 1, "")
			go s.watchGroupTests(job.ID, jobIDs)
		}
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"job_id": job.ID, "job_ids": jobIDs, "node_count": len(jobIDs)})
}

func (s *Server) watchGroupTests(parentID string, childIDs []string) {
	ctx := context.Background()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for range ticker.C {
		completed, failed := 0, 0
		for _, childID := range childIDs {
			child, err := s.store.JobRepo().FindByID(ctx, childID)
			if err != nil || !child.IsTerminal() {
				continue
			}
			completed++
			if child.Status == domain.JobFailed {
				failed++
			}
		}
		progress := completed * 100 / len(childIDs)
		if completed == len(childIDs) {
			status := domain.JobSucceeded
			message := ""
			if failed > 0 {
				status = domain.JobFailed
				message = fmt.Sprintf("%d child tests failed", failed)
			}
			_ = s.store.JobRepo().UpdateStatus(ctx, parentID, status, 100, message)
			return
		}
		_ = s.store.JobRepo().UpdateStatus(ctx, parentID, domain.JobRunning, progress, "")
	}
}
