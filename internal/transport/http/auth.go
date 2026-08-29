package httptransport

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	settingKeyAdminPasswordHash = "admin_password_hash"
	settingKeyAdminUsername     = "admin_username"
	sessionTTL                  = 24 * time.Hour
)

// sessionStore is an in-memory store for admin sessions.
type sessionStore struct {
	mu      sync.Mutex
	entries map[string]time.Time // token → expiry
}

func newSessionStore() *sessionStore { return &sessionStore{entries: make(map[string]time.Time)} }

func (ss *sessionStore) create() string {
	buf := make([]byte, 32)
	_, _ = rand.Read(buf)
	token := hex.EncodeToString(buf)
	ss.mu.Lock()
	ss.entries[token] = time.Now().Add(sessionTTL)
	ss.mu.Unlock()
	return token
}

func (ss *sessionStore) validate(token string) bool {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	exp, ok := ss.entries[token]
	if !ok {
		return false
	}
	if time.Now().After(exp) {
		delete(ss.entries, token)
		return false
	}
	return true
}

func (ss *sessionStore) revoke(token string) {
	ss.mu.Lock()
	delete(ss.entries, token)
	ss.mu.Unlock()
}

// hashPassword produces a hex-encoded SHA-256 digest of the plain-text password.
func hashPassword(password string) string {
	sum := sha256.Sum256([]byte(password))
	return hex.EncodeToString(sum[:])
}

// hashAPIKey produces a hex-encoded SHA-256 digest of the plain-text API key.
func hashAPIKey(key string) string { return hashPassword(key) }

// bearerToken extracts the token from an "Authorization: Bearer <token>" header.
// Returns empty string if the header is absent or malformed.
func bearerToken(r *http.Request) string {
	v := r.Header.Get("Authorization")
	if !strings.HasPrefix(v, "Bearer ") {
		return ""
	}
	return strings.TrimSpace(v[len("Bearer "):])
}

// adminRequired wraps a handler so that it requires a valid admin session token.
func (s *Server) adminRequired(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := bearerToken(r)
		if token == "" || !s.sessions.validate(token) {
			apiErr(w, http.StatusUnauthorized, "unauthorized", "valid admin session token required")
			return
		}
		next(w, r)
	}
}

// apikeyRequired wraps a handler so that it requires a valid, active API key.
// It injects the resolved domain.APIKey into the request context.
func (s *Server) apikeyRequired(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.requireStore(w) {
			return
		}
		token := bearerToken(r)
		if token == "" {
			apiErr(w, http.StatusUnauthorized, "unauthorized", "Bearer API key required")
			return
		}
		hash := hashAPIKey(token)
		key, err := s.store.APIKeyRepo().FindByKeyHash(r.Context(), hash)
		if err != nil || !key.IsActive(time.Now()) {
			apiErr(w, http.StatusUnauthorized, "unauthorized", "invalid or expired API key")
			return
		}
		ctx := context.WithValue(r.Context(), ctxKeyAPIKey{}, key)
		next(w, r.WithContext(ctx))
	}
}

type ctxKeyAPIKey struct{}
type ctxKeyAdminSession struct{}

// POST /api/v1/auth/login
func (s *Server) authLogin(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Password string `json:"password"`
	}
	if err := decodeJSON(r, &body); err != nil {
		apiErr(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}
	if body.Password == "" {
		apiErr(w, http.StatusBadRequest, "bad_request", "password required")
		return
	}

	expectedHash := ""
	if s.store != nil {
		expectedHash, _ = s.store.SettingsRepo().Get(r.Context(), settingKeyAdminPasswordHash)
	}
	if expectedHash == "" {
		apiErr(w, http.StatusServiceUnavailable, "admin_not_configured", "administrator password has not been configured")
		return
	}

	if hashPassword(body.Password) != expectedHash {
		apiErr(w, http.StatusUnauthorized, "unauthorized", "invalid password")
		return
	}

	token := s.sessions.create()
	writeJSON(w, http.StatusOK, map[string]any{
		"token":      token,
		"expires_in": int(sessionTTL.Seconds()),
	})
}

// POST /api/v1/auth/logout
func (s *Server) authLogout(w http.ResponseWriter, r *http.Request) {
	token := bearerToken(r)
	s.sessions.revoke(token)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// GET /api/v1/auth/me
func (s *Server) authMe(w http.ResponseWriter, r *http.Request) {
	username := "admin"
	if s.store != nil {
		if value, err := s.store.SettingsRepo().Get(r.Context(), settingKeyAdminUsername); err == nil && value != "" {
			username = value
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"username": username,
		"roles":    []string{"admin"},
	})
}

// PUT /api/v1/auth/password
func (s *Server) authChangePassword(w http.ResponseWriter, r *http.Request) {
	if !s.requireStore(w) {
		return
	}
	var body struct {
		OldPassword string `json:"old_password"`
		NewPassword string `json:"new_password"`
	}
	if err := decodeJSON(r, &body); err != nil {
		apiErr(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}
	if body.OldPassword == "" || body.NewPassword == "" {
		apiErr(w, http.StatusBadRequest, "bad_request", "old_password and new_password required")
		return
	}

	existing, _ := s.store.SettingsRepo().Get(r.Context(), settingKeyAdminPasswordHash)
	if existing == "" {
		apiErr(w, http.StatusServiceUnavailable, "admin_not_configured", "administrator password has not been configured")
		return
	}
	if hashPassword(body.OldPassword) != existing {
		apiErr(w, http.StatusUnauthorized, "unauthorized", "incorrect current password")
		return
	}

	if err := s.store.SettingsRepo().Set(r.Context(), settingKeyAdminPasswordHash, hashPassword(body.NewPassword)); err != nil {
		apiErr(w, http.StatusInternalServerError, "internal_error", "failed to update password")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
