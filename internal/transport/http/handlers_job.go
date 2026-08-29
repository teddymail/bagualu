package httptransport

import (
	"net/http"
	"strings"
	"time"

	"github.com/teddymail/bagualu/internal/domain"
)

// GET /api/v1/jobs
func (s *Server) listJobs(w http.ResponseWriter, r *http.Request) {
	if !s.requireStore(w) {
		return
	}
	q := r.URL.Query()
	jobs, err := s.store.JobRepo().FindActive(r.Context(), 0)
	if err != nil {
		apiErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	if kind := q.Get("kind"); kind != "" {
		filtered := jobs[:0]
		for _, job := range jobs {
			if job.Kind == kind {
				filtered = append(filtered, job)
			}
		}
		jobs = filtered
	}
	if status := domain.JobStatus(q.Get("status")); status != "" {
		filtered := jobs[:0]
		for _, job := range jobs {
			if job.Status == status {
				filtered = append(filtered, job)
			}
		}
		jobs = filtered
	}
	resp := make([]jobResponse, 0, len(jobs))
	for i := range jobs {
		resp = append(resp, toJobResponse(&jobs[i]))
	}
	writeJSON(w, http.StatusOK, map[string]any{"jobs": resp})
}

// POST /api/v1/jobs/clear
// Clears active test jobs while preserving all historical task records.
func (s *Server) clearJobs(w http.ResponseWriter, r *http.Request) {
	if !s.requireStore(w) {
		return
	}
	jScope := r.URL.Query().Get("scope")
	if jScope == "" {
		jScope = "tests"
	}
	jobs, err := s.store.JobRepo().FindActive(r.Context(), 0)
	if err != nil {
		apiErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	requested, cancelled, cancelRequested := 0, 0, 0
	for i := range jobs {
		job := &jobs[i]
		isTest := strings.HasPrefix(job.Kind, "test_")
		if jScope != "all" && !isTest {
			continue
		}
		requested++
		found := false
		if isTest && s.testCancel != nil {
			var cancelErr error
			found, cancelErr = s.testCancel(r.Context(), job.ID)
			if cancelErr != nil {
				apiErr(w, http.StatusInternalServerError, "clear_failed", cancelErr.Error())
				return
			}
		}
		updated, findErr := s.store.JobRepo().FindByID(r.Context(), job.ID)
		if findErr == nil && updated.IsTerminal() {
			cancelled++
			continue
		}
		if found {
			cancelRequested++
		}
		if err := s.store.JobRepo().UpdateStatus(r.Context(), job.ID, domain.JobCancelled, job.Progress, "cancelled by user"); err != nil {
			apiErr(w, http.StatusInternalServerError, "clear_failed", err.Error())
			return
		}
		cancelled++
	}
	if r.URL.Query().Get("purge") == "true" {
		if err := s.store.JobRepo().DeleteAll(r.Context()); err != nil {
			apiErr(w, http.StatusInternalServerError, "clear_failed", err.Error())
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status": "ok", "scope": jScope, "requested": requested,
		"cancelled": cancelled, "cancel_requested": cancelRequested,
		"purged": r.URL.Query().Get("purge") == "true",
	})
}

// GET /api/v1/jobs/{id}
func (s *Server) getJob(w http.ResponseWriter, r *http.Request) {
	if !s.requireStore(w) {
		return
	}
	id := r.PathValue("id")
	job, err := s.store.JobRepo().FindByID(r.Context(), id)
	if err != nil {
		writeNotFoundOrError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toJobResponse(job))
}

// POST /api/v1/jobs/{id}/cancel
func (s *Server) cancelJob(w http.ResponseWriter, r *http.Request) {
	if !s.requireStore(w) {
		return
	}
	id := r.PathValue("id")
	job, err := s.store.JobRepo().FindByID(r.Context(), id)
	if err != nil {
		writeNotFoundOrError(w, err)
		return
	}
	if job.IsTerminal() {
		apiErr(w, http.StatusConflict, "job_already_terminal", "job is already in a terminal state")
		return
	}
	if s.testCancel != nil {
		cancelled, cancelErr := s.testCancel(r.Context(), id)
		if cancelErr != nil {
			apiErr(w, http.StatusInternalServerError, "cancel_failed", cancelErr.Error())
			return
		}
		if cancelled {
			job, err = s.store.JobRepo().FindByID(r.Context(), id)
			if err != nil {
				writeNotFoundOrError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, toJobResponse(job))
			return
		}
	}
	if err := s.store.JobRepo().UpdateStatus(r.Context(), id, domain.JobCancelled, job.Progress, "cancelled by user"); err != nil {
		apiErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	job.Status = domain.JobCancelled
	now := time.Now().UTC()
	job.FinishedAt = &now
	writeJSON(w, http.StatusOK, toJobResponse(job))
}
