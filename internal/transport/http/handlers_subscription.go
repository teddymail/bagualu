package httptransport

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/teddymail/bagualu/internal/domain"
	"github.com/teddymail/bagualu/internal/modules/resource"
	"github.com/teddymail/bagualu/internal/modules/subscription_output"
)

// generateToken creates a secure random subscription token prefixed with "sl_".
func generateToken() string {
	b := make([]byte, 24)
	_, _ = rand.Read(b)
	return "sl_" + hex.EncodeToString(b)
}

// GET /api/v1/subscription-links
func (s *Server) listSubscriptionLinks(w http.ResponseWriter, r *http.Request) {
	if !s.requireStore(w) {
		return
	}
	links, err := s.store.SubscriptionLinkRepo().FindAll(r.Context())
	if err != nil {
		apiErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	resp := make([]subscriptionLinkResponse, 0, len(links))
	for i := range links {
		resp = append(resp, toSubscriptionLinkResponse(&links[i]))
	}
	writeJSON(w, http.StatusOK, map[string]any{"subscription_links": resp})
}

// POST /api/v1/subscription-links
func (s *Server) createSubscriptionLink(w http.ResponseWriter, r *http.Request) {
	if !s.requireStore(w) {
		return
	}
	var body struct {
		Name           string     `json:"name"`
		GroupID        string     `json:"group_id"`
		DefaultFormat  string     `json:"default_format"`
		AllowedFormats []string   `json:"allowed_formats"`
		MinScore       float64    `json:"min_score"`
		Limit          int        `json:"limit"`
		HealthyOnly    bool       `json:"healthy_only"`
		Enabled        bool       `json:"enabled"`
		ExpiresAt      *time.Time `json:"expires_at"`
	}
	if err := decodeJSON(r, &body); err != nil {
		apiErr(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}
	if body.Name == "" || body.GroupID == "" {
		apiErr(w, http.StatusBadRequest, "bad_request", "name and group_id are required")
		return
	}
	if body.DefaultFormat == "" {
		body.DefaultFormat = "clash"
	}
	plainToken := generateToken()
	now := time.Now().UTC()
	l := &domain.SubscriptionLink{
		ID:             uuid.NewString(),
		Name:           body.Name,
		GroupID:        body.GroupID,
		TokenHash:      hashAPIKey(plainToken),
		TokenPrefix:    plainToken[:10],
		DefaultFormat:  body.DefaultFormat,
		AllowedFormats: normalizeAllowedFormats(body.AllowedFormats),
		MinScore:       body.MinScore,
		Limit:          body.Limit,
		HealthyOnly:    body.HealthyOnly,
		Enabled:        body.Enabled,
		ExpiresAt:      body.ExpiresAt,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := s.store.SubscriptionLinkRepo().Save(r.Context(), l); err != nil {
		apiErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"subscription_link": toSubscriptionLinkResponse(l),
		"token":             plainToken,
	})
}

// GET /api/v1/subscription-links/{id}
func (s *Server) getSubscriptionLink(w http.ResponseWriter, r *http.Request) {
	if !s.requireStore(w) {
		return
	}
	id := r.PathValue("id")
	l, err := s.store.SubscriptionLinkRepo().FindByID(r.Context(), id)
	if err != nil {
		writeNotFoundOrError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toSubscriptionLinkResponse(l))
}

// PUT /api/v1/subscription-links/{id}
func (s *Server) updateSubscriptionLink(w http.ResponseWriter, r *http.Request) {
	if !s.requireStore(w) {
		return
	}
	id := r.PathValue("id")
	l, err := s.store.SubscriptionLinkRepo().FindByID(r.Context(), id)
	if err != nil {
		writeNotFoundOrError(w, err)
		return
	}
	var body struct {
		Name           *string    `json:"name"`
		DefaultFormat  *string    `json:"default_format"`
		AllowedFormats *[]string  `json:"allowed_formats"`
		MinScore       *float64   `json:"min_score"`
		Limit          *int       `json:"limit"`
		HealthyOnly    *bool      `json:"healthy_only"`
		Enabled        *bool      `json:"enabled"`
		ExpiresAt      *time.Time `json:"expires_at"`
	}
	if err := decodeJSON(r, &body); err != nil {
		apiErr(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}
	if body.Name != nil {
		l.Name = *body.Name
	}
	if body.DefaultFormat != nil {
		l.DefaultFormat = *body.DefaultFormat
	}
	if body.AllowedFormats != nil {
		l.AllowedFormats = normalizeAllowedFormats(*body.AllowedFormats)
	}
	if body.MinScore != nil {
		l.MinScore = *body.MinScore
	}
	if body.Limit != nil {
		l.Limit = *body.Limit
	}
	if body.HealthyOnly != nil {
		l.HealthyOnly = *body.HealthyOnly
	}
	if body.Enabled != nil {
		l.Enabled = *body.Enabled
	}
	if body.ExpiresAt != nil {
		l.ExpiresAt = body.ExpiresAt
	}
	l.UpdatedAt = time.Now().UTC()
	if err := s.store.SubscriptionLinkRepo().Save(r.Context(), l); err != nil {
		apiErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, toSubscriptionLinkResponse(l))
}

// DELETE /api/v1/subscription-links/{id}
func (s *Server) deleteSubscriptionLink(w http.ResponseWriter, r *http.Request) {
	if !s.requireStore(w) {
		return
	}
	id := r.PathValue("id")
	if err := s.store.SubscriptionLinkRepo().Delete(r.Context(), id); err != nil {
		writeNotFoundOrError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// POST /api/v1/subscription-links/{id}/rotate
func (s *Server) rotateSubscriptionLink(w http.ResponseWriter, r *http.Request) {
	if !s.requireStore(w) {
		return
	}
	id := r.PathValue("id")
	l, err := s.store.SubscriptionLinkRepo().FindByID(r.Context(), id)
	if err != nil {
		writeNotFoundOrError(w, err)
		return
	}
	plainToken := generateToken()
	l.TokenHash = hashAPIKey(plainToken)
	l.TokenPrefix = plainToken[:10]
	l.UpdatedAt = time.Now().UTC()
	if err := s.store.SubscriptionLinkRepo().Save(r.Context(), l); err != nil {
		apiErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"subscription_link": toSubscriptionLinkResponse(l),
		"token":             plainToken,
	})
}

// GET /api/v1/subscription-links/{id}/preview
func (s *Server) previewSubscriptionLink(w http.ResponseWriter, r *http.Request) {
	if !s.requireStore(w) {
		return
	}
	id := r.PathValue("id")
	l, err := s.store.SubscriptionLinkRepo().FindByID(r.Context(), id)
	if err != nil {
		writeNotFoundOrError(w, err)
		return
	}
	format := r.URL.Query().Get("format")
	if format == "" {
		format = l.DefaultFormat
	}
	if format == "daed" {
		format = string(subscription_output.FormatDAE)
	}
	if !formatAllowed(subscription_output.Format(format), l.AllowedFormats) {
		apiErr(w, http.StatusForbidden, "subscription_format_forbidden", "format is not allowed for this subscription link")
		return
	}
	nodes, skipped := s.filterGroupNodesWithPolicy(r, l.GroupID, l.MinScore, l.Limit, l.HealthyOnly)
	preview := subscription_output.Preview(nodes, subscription_output.Format(format))
	allSkipped := append(skipped, preview.Skipped...)
	writeJSON(w, http.StatusOK, map[string]any{
		"format":           format,
		"node_count":       preview.CompatibleCount,
		"compatible_count": preview.CompatibleCount,
		"skipped":          allSkipped,
		"skipped_count":    len(allSkipped),
		"regions":          preview.Regions,
		"uris":             preview.URIs,
		"group_id":         l.GroupID,
		"min_score":        l.MinScore,
		"healthy_only":     l.HealthyOnly,
		"allowed_formats":  l.AllowedFormats,
	})
}

// POST /api/v1/subscription-links/{id}/detect-preview
func (s *Server) detectPreviewSubscriptionLink(w http.ResponseWriter, r *http.Request) {
	var body struct {
		UserAgent string `json:"user_agent"`
	}
	if err := decodeJSON(r, &body); err != nil {
		apiErr(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}
	client, format, ambiguous := subscription_output.DetectUserAgent(body.UserAgent)
	writeJSON(w, http.StatusOK, map[string]any{
		"client":    client,
		"format":    string(format),
		"ambiguous": ambiguous,
	})
}

// GET /api/v1/client-detection-rules
func (s *Server) getClientDetectionRules(w http.ResponseWriter, r *http.Request) {
	rules := defaultClientDetectionRules()
	if s.store != nil {
		if raw, err := s.store.SettingsRepo().Get(r.Context(), "client_detection_rules"); err == nil && raw != "" {
			_ = json.Unmarshal([]byte(raw), &rules)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"rules": rules,
	})
}

// PUT /api/v1/client-detection-rules
func (s *Server) putClientDetectionRules(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	if err := decodeJSON(r, &body); err != nil {
		apiErr(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}
	if s.store != nil {
		encoded, err := json.Marshal(body["rules"])
		if err != nil || s.store.SettingsRepo().Set(r.Context(), "client_detection_rules", string(encoded)) != nil {
			apiErr(w, http.StatusInternalServerError, "internal_error", "failed to save detection rules")
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// GET /api/v1/score-policy
func (s *Server) getScorePolicy(w http.ResponseWriter, r *http.Request) {
	policy := domain.DefaultScorePolicy()
	if s.scorePolicyGet != nil {
		policy = s.scorePolicyGet()
	}
	writeJSON(w, http.StatusOK, policy)
}

// PUT /api/v1/score-policy
func (s *Server) putScorePolicy(w http.ResponseWriter, r *http.Request) {
	var policy domain.ScorePolicy
	if err := decodeJSON(r, &policy); err != nil {
		apiErr(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}
	if s.scorePolicySet != nil {
		if err := s.scorePolicySet(policy); err != nil {
			apiErr(w, http.StatusBadRequest, "invalid_score_policy", err.Error())
			return
		}
	}
	writeJSON(w, http.StatusOK, policy)
}

// GET /api/v1/regions
func (s *Server) listRegions(w http.ResponseWriter, r *http.Request) {
	if !s.requireStore(w) {
		return
	}
	nodes, _ := s.store.NodeRepo().FindAll(r.Context(), domainNodeFilter{})
	regionCounts := map[string]int{}
	for _, n := range nodes {
		if n.Region != "" {
			regionCounts[n.Region]++
		}
	}
	regions := make([]map[string]any, 0, len(regionCounts))
	for code, count := range regionCounts {
		displayName, enabled := code, true
		if s.store != nil {
			if raw, err := s.store.SettingsRepo().Get(r.Context(), "region:"+code); err == nil && raw != "" {
				var saved struct {
					DisplayName string `json:"display_name"`
					Enabled     *bool  `json:"enabled"`
				}
				if json.Unmarshal([]byte(raw), &saved) == nil {
					if saved.DisplayName != "" {
						displayName = saved.DisplayName
					}
					if saved.Enabled != nil {
						enabled = *saved.Enabled
					}
				}
			}
		}
		regions = append(regions, map[string]any{
			"code":         code,
			"node_count":   count,
			"display_name": displayName,
			"enabled":      enabled,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"regions": regions})
}

// PUT /api/v1/regions/{code}
func (s *Server) updateRegion(w http.ResponseWriter, r *http.Request) {
	code := r.PathValue("code")
	var body struct {
		DisplayName *string `json:"display_name"`
		Enabled     *bool   `json:"enabled"`
	}
	if err := decodeJSON(r, &body); err != nil {
		apiErr(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}
	if s.store != nil {
		encoded, err := json.Marshal(body)
		if err != nil || s.store.SettingsRepo().Set(r.Context(), "region:"+code, string(encoded)) != nil {
			apiErr(w, http.StatusInternalServerError, "internal_error", "failed to save region")
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"code": code, "status": "ok"})
}

func defaultClientDetectionRules() []map[string]any {
	return []map[string]any{
		{"id": "sing-box", "pattern": `sing-box|sfa|sfi|sfm`, "format": "sing-box", "priority": 10},
		{"id": "dae", "pattern": `dae-wing|daed|dae`, "format": "dae", "priority": 20},
		{"id": "base64", "pattern": `v2rayng|v2rayn|passwall|shadowsocksr|shadowrocket|nekobox`, "format": "base64", "priority": 30},
		{"id": "clash", "pattern": `clashverge|mihomo|openclash|nikki|clash`, "format": "clash", "priority": 40},
		{"id": "json", "pattern": `api|json`, "format": "json", "priority": 50},
	}
}

// filterGroupNodes applies resource filter rules and returns nodes + skip reasons.
func (s *Server) filterGroupNodes(r *http.Request, groupID string, minScore float64, limit int) ([]domain.Node, []string) {
	return s.filterGroupNodesWithPolicy(r, groupID, minScore, limit, true)
}

func (s *Server) filterGroupNodesWithPolicy(r *http.Request, groupID string, minScore float64, limit int, healthyOnly bool) ([]domain.Node, []string) {
	if s.store == nil {
		return nil, nil
	}
	nodeIDs, err := s.store.GroupRepo().FindNodeIDs(r.Context(), groupID)
	if err != nil {
		return nil, nil
	}
	nodes := make([]domain.Node, 0, len(nodeIDs))
	var skipped []string
	for _, id := range nodeIDs {
		n, err := s.store.NodeRepo().FindByID(r.Context(), id)
		if err != nil {
			skipped = append(skipped, id+": not found")
			continue
		}
		snapshot, _ := s.store.ScoreSnapshotRepo().FindLatestByNodeID(r.Context(), id)
		if snapshot != nil {
			score := snapshot.ToScore()
			n.Score = &score
		}
		nodes = append(nodes, *n)
	}
	if group, err := s.store.GroupRepo().FindByID(r.Context(), groupID); err == nil && group.MinScore > minScore {
		minScore = group.MinScore
	}
	q := r.URL.Query()
	if value, err := strconv.ParseFloat(q.Get("min_score"), 64); err == nil && value > minScore {
		minScore = value
	}
	if value, err := strconv.Atoi(q.Get("limit")); err == nil && value > 0 && (limit == 0 || value < limit) {
		limit = value
	}
	if value := q.Get("healthy_only"); value != "" {
		healthyOnly = value != "false" && value != "0"
	}
	if healthyOnly {
		kept := nodes[:0]
		for _, node := range nodes {
			if node.Status == domain.NodeActive {
				kept = append(kept, node)
				continue
			}
			skipped = append(skipped, node.ID+": unhealthy status")
		}
		nodes = kept
	}
	group, _ := s.store.GroupRepo().FindByID(r.Context(), groupID)
	filtered, selectionSkipped := resource.SelectWithReasons(nodes, resource.Filter{
		Protocol: q.Get("protocol"), Region: q.Get("region"), Tag: q.Get("tag"), Sort: q.Get("sort"),
		MinScore: minScore, Limit: limit,
		OnePerEndpointIP: group != nil && group.OnePerEndpointIP,
	})
	for _, skippedNode := range selectionSkipped {
		skipped = append(skipped, skippedNode.NodeID+": "+skippedNode.Reason)
	}
	return filtered, skipped
}

func normalizeAllowedFormats(values []string) []string {
	seen := map[string]bool{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		format := strings.TrimSpace(value)
		if format == "daed" {
			format = string(subscription_output.FormatDAE)
		}
		if !supportedFormat(subscription_output.Format(format)) || seen[format] {
			continue
		}
		seen[format] = true
		result = append(result, format)
	}
	return result
}

func formatAllowed(format subscription_output.Format, allowed []string) bool {
	if format == "daed" {
		format = subscription_output.FormatDAE
	}
	if !supportedFormat(format) || len(allowed) == 0 {
		return supportedFormat(format)
	}
	for _, value := range allowed {
		if subscription_output.Format(value) == format {
			return true
		}
	}
	return false
}
