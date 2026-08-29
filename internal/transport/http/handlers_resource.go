package httptransport

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/teddymail/bagualu/internal/domain"
	"github.com/teddymail/bagualu/internal/modules/subscription_output"
)

// GET /api/v1/resources  – returns default group for the API key
func (s *Server) resourcesDefault(w http.ResponseWriter, r *http.Request) {
	key := r.Context().Value(ctxKeyAPIKey{}).(*domain.APIKey)
	if key.GroupID == "" {
		apiErr(w, http.StatusNotFound, "no_default_group", "API key has no default group")
		return
	}
	s.serveGroupResource(w, r, key.GroupID)
}

// GET /api/v1/resources/{group}
func (s *Server) resourcesGroup(w http.ResponseWriter, r *http.Request) {
	if !s.groupAllowed(r, r.PathValue("group")) {
		apiErr(w, http.StatusForbidden, "group_forbidden", "API key is not authorized for this group")
		return
	}
	s.serveGroupResource(w, r, r.PathValue("group"))
}

// GET /api/v1/resources/{group}/nodes
func (s *Server) resourcesGroupNodes(w http.ResponseWriter, r *http.Request) {
	groupID := r.PathValue("group")
	if !s.groupAllowed(r, groupID) {
		apiErr(w, http.StatusForbidden, "group_forbidden", "API key is not authorized for this group")
		return
	}
	nodes, _ := s.filterGroupNodes(r, groupID, 60, 0)
	writeJSON(w, http.StatusOK, map[string]any{"nodes": toNodeResponseList(nodes)})
}

// GET /api/v1/resources/{group}/subscription
func (s *Server) resourcesGroupSubscription(w http.ResponseWriter, r *http.Request) {
	groupID := r.PathValue("group")
	if !s.groupAllowed(r, groupID) {
		apiErr(w, http.StatusForbidden, "group_forbidden", "API key is not authorized for this group")
		return
	}
	format, source := resolveFormat(r, "")
	nodes, _ := s.filterGroupNodes(r, groupID, 60, 0)
	s.renderSubscription(w, r, nodes, format, source)
}

// GET /api/v1/resources/{group}/stats
func (s *Server) resourcesGroupStats(w http.ResponseWriter, r *http.Request) {
	groupID := r.PathValue("group")
	if !s.groupAllowed(r, groupID) {
		apiErr(w, http.StatusForbidden, "group_forbidden", "API key is not authorized for this group")
		return
	}
	if !s.requireStore(w) {
		return
	}
	nodeIDs, _ := s.store.GroupRepo().FindNodeIDs(r.Context(), groupID)
	nodes, _ := s.filterGroupNodes(r, groupID, 60, 0)
	writeJSON(w, http.StatusOK, map[string]any{
		"group_id":        groupID,
		"total_nodes":     len(nodeIDs),
		"available_nodes": len(nodes),
	})
}

// GET /api/v1/subscribe/{token}
func (s *Server) subscribeByToken(w http.ResponseWriter, r *http.Request) {
	if !s.requireStore(w) {
		return
	}
	token := r.PathValue("token")
	hash := hashAPIKey(token)
	link, err := s.store.SubscriptionLinkRepo().FindByTokenHash(r.Context(), hash)
	if err != nil || !link.IsAccessible(time.Now()) {
		apiErr(w, http.StatusUnauthorized, "unauthorized", "invalid or expired subscription token")
		return
	}

	// Use a request-independent context so access accounting is not lost when
	// the client closes the connection after receiving the subscription.
	accessCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_ = s.store.SubscriptionLinkRepo().UpdateLastAccess(accessCtx, link.ID, time.Now().UTC())

	format, source := resolveFormat(r, link.DefaultFormat)
	if !formatAllowed(format, link.AllowedFormats) {
		if r.URL.Query().Get("format") != "" && supportedFormat(format) {
			apiErr(w, http.StatusForbidden, "subscription_format_forbidden", "format is not allowed for this subscription link")
			return
		}
		apiErr(w, http.StatusBadRequest, "unsupported_subscription_format", "unsupported subscription format")
		return
	}
	nodes, _ := s.filterGroupNodesWithPolicy(r, link.GroupID, link.MinScore, link.Limit, link.HealthyOnly)
	s.renderSubscription(w, r, nodes, format, source)
}

// serveGroupResource renders the subscription for a group using the caller's format preference.
func (s *Server) serveGroupResource(w http.ResponseWriter, r *http.Request, groupID string) {
	format, source := resolveFormat(r, "clash")
	if !supportedFormat(format) {
		apiErr(w, http.StatusBadRequest, "unsupported_subscription_format", "unsupported subscription format")
		return
	}
	nodes, _ := s.filterGroupNodes(r, groupID, 60, 0)
	s.renderSubscription(w, r, nodes, format, source)
}

// resolveFormat selects the output format following SDD §10.4 priority rules:
//  1. Explicit ?format= query parameter
//  2. User-Agent detection
//  3. linkDefault (subscription link's configured default)
func resolveFormat(r *http.Request, linkDefault string) (format subscription_output.Format, source string) {
	if q := r.URL.Query().Get("format"); q != "" {
		return subscription_output.Format(q), "query"
	}
	ua := r.Header.Get("User-Agent")
	if ua != "" {
		_, detected, ambiguous := subscription_output.DetectUserAgent(ua)
		if !ambiguous && detected != "" {
			return detected, "user-agent"
		}
	}
	if linkDefault != "" {
		return subscription_output.Format(linkDefault), "default"
	}
	return subscription_output.FormatClash, "default"
}

// renderSubscription renders nodes into the requested format and writes the response.
func (s *Server) renderSubscription(
	w http.ResponseWriter,
	r *http.Request,
	nodes []domain.Node,
	format subscription_output.Format,
	source string,
) {
	if format == "daed" {
		format = subscription_output.FormatDAE
	}

	if source != "query" {
		w.Header().Set("Vary", "User-Agent")
	}
	client := "not_evaluated"
	if source != "query" {
		client, _, _ = subscription_output.DetectUserAgent(r.Header.Get("User-Agent"))
	}
	w.Header().Set("X-Bagualu-Client", client)

	// Diagnostic headers per SDD §10.4
	w.Header().Set("X-Bagualu-Subscription-Format", string(format))
	w.Header().Set("X-Bagualu-Format-Source", source)

	preview := subscription_output.Preview(nodes, format)
	data, err := subscription_output.Render(nodes, subscription_output.RenderOptions{Format: format})
	if err != nil {
		code := "subscription_no_compatible_nodes"
		if !supportedFormat(format) {
			code = "unsupported_subscription_format"
		}
		apiErr(w, http.StatusUnprocessableEntity, code, err.Error())
		return
	}

	etagBytes := sha256.Sum256(data)
	etag := `"` + hex.EncodeToString(etagBytes[:]) + `"`
	w.Header().Set("ETag", etag)
	lastModified := time.Time{}
	for _, node := range nodes {
		if node.UpdatedAt.After(lastModified) {
			lastModified = node.UpdatedAt
		}
	}
	if lastModified.IsZero() {
		lastModified = time.Now().UTC()
	}
	w.Header().Set("Last-Modified", lastModified.UTC().Format(http.TimeFormat))
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Bagualu-Node-Count", strconv.Itoa(preview.CompatibleCount))
	w.Header().Set("X-Bagualu-Skipped-Count", strconv.Itoa(len(preview.Skipped)))
	w.Header().Set("Content-Disposition", "inline; filename*=UTF-8''"+url.PathEscape("八卦炉-subscription."+extensionForFormat(format)))
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	ct := contentTypeForFormat(format)
	writeText(w, http.StatusOK, ct, data)
}

func supportedFormat(format subscription_output.Format) bool {
	switch format {
	case subscription_output.FormatClash, subscription_output.FormatBase64,
		subscription_output.FormatSingBox, subscription_output.FormatDAE,
		subscription_output.FormatDAED, subscription_output.FormatJSON,
		subscription_output.FormatOriginal:
		return true
	default:
		return false
	}
}

func (s *Server) groupAllowed(r *http.Request, groupID string) bool {
	key, ok := r.Context().Value(ctxKeyAPIKey{}).(*domain.APIKey)
	return ok && key != nil && key.GroupID == groupID
}

func contentTypeForFormat(format subscription_output.Format) string {
	switch format {
	case subscription_output.FormatClash:
		return "text/yaml; charset=utf-8"
	case subscription_output.FormatSingBox:
		return "application/json; charset=utf-8"
	case subscription_output.FormatJSON:
		return "application/json; charset=utf-8"
	default:
		return "text/plain; charset=utf-8"
	}
}

func extensionForFormat(format subscription_output.Format) string {
	switch format {
	case subscription_output.FormatClash:
		return "yaml"
	case subscription_output.FormatSingBox, subscription_output.FormatJSON:
		return "json"
	default:
		return "txt"
	}
}
