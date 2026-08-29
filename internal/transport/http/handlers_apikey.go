package httptransport

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/teddymail/bagualu/internal/domain"
)

// generateAPIKey creates a secure random API key prefixed with "bg_".
func generateAPIKey() string {
	b := make([]byte, 24)
	_, _ = rand.Read(b)
	return "bg_" + hex.EncodeToString(b)
}

// GET /api/v1/api-keys
func (s *Server) listAPIKeys(w http.ResponseWriter, r *http.Request) {
	if !s.requireStore(w) {
		return
	}
	keys, err := s.store.APIKeyRepo().FindAll(r.Context())
	if err != nil {
		apiErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	resp := make([]apiKeyResponse, 0, len(keys))
	for i := range keys {
		resp = append(resp, toAPIKeyResponse(&keys[i]))
	}
	writeJSON(w, http.StatusOK, map[string]any{"api_keys": resp})
}

// POST /api/v1/api-keys
func (s *Server) createAPIKey(w http.ResponseWriter, r *http.Request) {
	if !s.requireStore(w) {
		return
	}
	var body struct {
		Name      string     `json:"name"`
		GroupID   string     `json:"group_id"`
		ExpiresAt *time.Time `json:"expires_at"`
	}
	if err := decodeJSON(r, &body); err != nil {
		apiErr(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}
	if body.Name == "" {
		apiErr(w, http.StatusBadRequest, "bad_request", "name is required")
		return
	}
	plainKey := generateAPIKey()
	now := time.Now().UTC()
	k := &domain.APIKey{
		ID:        uuid.NewString(),
		Name:      body.Name,
		GroupID:   body.GroupID,
		KeyHash:   hashAPIKey(plainKey),
		Prefix:    plainKey[:10],
		ExpiresAt: body.ExpiresAt,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := s.store.APIKeyRepo().Save(r.Context(), k); err != nil {
		apiErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	// Return the full key only on creation
	writeJSON(w, http.StatusCreated, map[string]any{
		"api_key": toAPIKeyResponse(k),
		"key":     plainKey, // only returned here
	})
}

// PUT /api/v1/api-keys/{id}
func (s *Server) updateAPIKey(w http.ResponseWriter, r *http.Request) {
	if !s.requireStore(w) {
		return
	}
	id := r.PathValue("id")
	k, err := s.store.APIKeyRepo().FindByID(r.Context(), id)
	if err != nil {
		writeNotFoundOrError(w, err)
		return
	}
	var body struct {
		Name      *string    `json:"name"`
		GroupID   *string    `json:"group_id"`
		ExpiresAt *time.Time `json:"expires_at"`
	}
	if err := decodeJSON(r, &body); err != nil {
		apiErr(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}
	if body.Name != nil {
		k.Name = *body.Name
	}
	if body.GroupID != nil {
		k.GroupID = *body.GroupID
	}
	if body.ExpiresAt != nil {
		k.ExpiresAt = body.ExpiresAt
	}
	k.UpdatedAt = time.Now().UTC()
	if err := s.store.APIKeyRepo().Save(r.Context(), k); err != nil {
		apiErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, toAPIKeyResponse(k))
}

// POST /api/v1/api-keys/{id}/rotate
func (s *Server) rotateAPIKey(w http.ResponseWriter, r *http.Request) {
	if !s.requireStore(w) {
		return
	}
	id := r.PathValue("id")
	k, err := s.store.APIKeyRepo().FindByID(r.Context(), id)
	if err != nil {
		writeNotFoundOrError(w, err)
		return
	}
	plainKey := generateAPIKey()
	k.KeyHash = hashAPIKey(plainKey)
	k.Prefix = plainKey[:10]
	k.UpdatedAt = time.Now().UTC()
	if err := s.store.APIKeyRepo().Save(r.Context(), k); err != nil {
		apiErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"api_key": toAPIKeyResponse(k),
		"key":     plainKey,
	})
}

// POST /api/v1/api-keys/{id}/revoke
func (s *Server) revokeAPIKey(w http.ResponseWriter, r *http.Request) {
	if !s.requireStore(w) {
		return
	}
	id := r.PathValue("id")
	if err := s.store.APIKeyRepo().Revoke(r.Context(), id, time.Now().UTC()); err != nil {
		writeNotFoundOrError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
