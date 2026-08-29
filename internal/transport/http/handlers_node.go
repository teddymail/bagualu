package httptransport

import (
	"errors"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/teddymail/bagualu/internal/domain"
	"github.com/teddymail/bagualu/internal/modules/subscription_output"
)

func (s *Server) createNode(w http.ResponseWriter, r *http.Request) {
	if !s.requireStore(w) {
		return
	}
	var body struct {
		URI        string         `json:"uri"`
		Name       string         `json:"name"`
		Protocol   string         `json:"protocol"`
		Address    string         `json:"address"`
		EndpointIP string         `json:"endpoint_ip"`
		Region     string         `json:"region"`
		SourceURL  string         `json:"source_url"`
		Port       int            `json:"port"`
		RawConfig  map[string]any `json:"raw_config"`
	}
	if err := decodeJSON(r, &body); err != nil {
		apiErr(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}
	var node *domain.Node
	if strings.TrimSpace(body.URI) != "" {
		parsed, parseErr := subscription_output.Parse([]byte(strings.TrimSpace(body.URI)), "manual")
		if parseErr != nil || len(parsed.Nodes) != 1 {
			apiErr(w, http.StatusBadRequest, "bad_request", "uri must contain exactly one supported node")
			return
		}
		parsedNode := parsed.Nodes[0]
		parsedNode.ID = uuid.NewString()
		node = &parsedNode
	} else if strings.TrimSpace(body.Protocol) == "" || strings.TrimSpace(body.Address) == "" || body.Port < 1 || body.Port > 65535 {
		apiErr(w, http.StatusBadRequest, "bad_request", "protocol, address and valid port are required")
		return
	}
	now := time.Now().UTC()
	if node == nil {
		node = &domain.Node{ID: uuid.NewString(), Protocol: strings.ToLower(strings.TrimSpace(body.Protocol)),
			Address: strings.TrimSpace(body.Address), Port: body.Port, RawConfig: body.RawConfig}
	}
	if body.Name != "" {
		node.Name = strings.TrimSpace(body.Name)
	}
	if node.Name == "" {
		node.Name = node.Protocol + "-" + node.Address
	}
	if body.EndpointIP != "" {
		node.EndpointIP = strings.TrimSpace(body.EndpointIP)
	}
	if body.Region != "" {
		node.Region = strings.TrimSpace(body.Region)
	}
	node.SourceURL = "manual"
	node.Status = domain.NodeActive
	node.CreatedAt = now
	node.UpdatedAt = now
	if err := s.store.NodeRepo().Save(r.Context(), node); err != nil {
		apiErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, toNodeResponse(node))
}

// GET /api/v1/nodes
func (s *Server) listNodes(w http.ResponseWriter, r *http.Request) {
	if !s.requireStore(w) {
		return
	}
	q := r.URL.Query()
	f := domain.NodeFilter{
		Status:   domain.NodeStatus(q.Get("status")),
		Protocol: q.Get("protocol"),
		Region:   q.Get("region"),
		GroupID:  q.Get("group_id"),
		Search:   q.Get("search"),
		Sort:     q.Get("sort"),
	}
	if v, err := strconv.Atoi(q.Get("limit")); err == nil && v > 0 {
		f.Limit = v
	}
	if v, err := strconv.Atoi(q.Get("offset")); err == nil && v >= 0 {
		f.Offset = v
	}
	minScore, _ := strconv.ParseFloat(q.Get("min_score"), 64)
	scoreStatus := q.Get("score_status")
	scoreQuery := minScore > 0 || scoreStatus != "" || isScoreSort(q.Get("sort"))
	if scoreQuery {
		f.Limit = 0
		f.Offset = 0
		f.Sort = ""
	}
	nodes, err := s.store.NodeRepo().FindAll(r.Context(), f)
	if err != nil {
		apiErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	// Enrich nodes with latest score snapshot
	enrichNodesWithScore(r, s, nodes)
	total := len(nodes)
	if scoreQuery {
		filtered := nodes[:0]
		for _, node := range nodes {
			if minScore > 0 && (node.Score == nil || node.Score.Overall < minScore) {
				continue
			}
			if scoreStatus != "" && (node.Score == nil || string(node.Score.Status) != scoreStatus) {
				continue
			}
			filtered = append(filtered, node)
		}
		nodes = filtered
		total = len(nodes)
		sortNodesByScore(nodes, q.Get("sort"))
		if f.Offset > len(nodes) {
			f.Offset = len(nodes)
		}
		end := len(nodes)
		if f.Limit > 0 && f.Offset+f.Limit < end {
			end = f.Offset + f.Limit
		}
		nodes = nodes[f.Offset:end]
	}
	writeJSON(w, http.StatusOK, map[string]any{"nodes": toNodeResponseList(nodes), "total": total})
}

func isScoreSort(value string) bool {
	return value == "score" || value == "speed" || value == "latency" || value == "availability"
}

func sortNodesByScore(nodes []domain.Node, sortKey string) {
	if !isScoreSort(sortKey) {
		return
	}
	sort.SliceStable(nodes, func(left, right int) bool {
		var leftScore, rightScore domain.Score
		if nodes[left].Score != nil {
			leftScore = *nodes[left].Score
		}
		if nodes[right].Score != nil {
			rightScore = *nodes[right].Score
		}
		switch sortKey {
		case "speed":
			if leftScore.Speed != rightScore.Speed {
				return leftScore.Speed > rightScore.Speed
			}
		case "latency":
			if leftScore.Latency != rightScore.Latency {
				return leftScore.Latency < rightScore.Latency
			}
		case "availability":
			if leftScore.Availability != rightScore.Availability {
				return leftScore.Availability > rightScore.Availability
			}
		default:
			if leftScore.Overall != rightScore.Overall {
				return leftScore.Overall > rightScore.Overall
			}
		}
		return nodes[left].ID < nodes[right].ID
	})
}

// GET /api/v1/nodes/{id}
func (s *Server) getNode(w http.ResponseWriter, r *http.Request) {
	if !s.requireStore(w) {
		return
	}
	id := r.PathValue("id")
	node, err := s.store.NodeRepo().FindByID(r.Context(), id)
	if err != nil {
		writeNotFoundOrError(w, err)
		return
	}
	snapshot, _ := s.store.ScoreSnapshotRepo().FindLatestByNodeID(r.Context(), id)
	if snapshot != nil {
		score := snapshot.ToScore()
		node.Score = &score
	}
	rawSources, _ := s.store.NodeRepo().FindNodeSources(r.Context(), id)
	sources := make([]nodeSourceResponse, 0, len(rawSources))
	for _, src := range rawSources {
		sources = append(sources, toNodeSourceResponse(src))
	}
	writeJSON(w, http.StatusOK, map[string]any{"node": toNodeResponse(node), "sources": sources})
}

// PUT /api/v1/nodes/{id}
func (s *Server) updateNode(w http.ResponseWriter, r *http.Request) {
	if !s.requireStore(w) {
		return
	}
	id := r.PathValue("id")
	node, err := s.store.NodeRepo().FindByID(r.Context(), id)
	if err != nil {
		writeNotFoundOrError(w, err)
		return
	}
	sources, sourceErr := s.store.NodeRepo().FindNodeSources(r.Context(), id)
	if sourceErr != nil {
		apiErr(w, http.StatusInternalServerError, "internal_error", sourceErr.Error())
		return
	}
	if len(sources) > 0 {
		apiErr(w, http.StatusConflict, "subscription_node_managed", "subscription nodes are managed by their upstream")
		return
	}
	var body struct {
		URI    *string `json:"uri"`
		Name   *string `json:"name"`
		Region *string `json:"region"`
	}
	if err := decodeJSON(r, &body); err != nil {
		apiErr(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}
	if body.URI != nil && strings.TrimSpace(*body.URI) != "" {
		parsed, parseErr := subscription_output.Parse([]byte(strings.TrimSpace(*body.URI)), "manual")
		if parseErr != nil || len(parsed.Nodes) != 1 {
			apiErr(w, http.StatusBadRequest, "bad_request", "uri must contain exactly one supported node")
			return
		}
		parsedNode := parsed.Nodes[0]
		parsedNode.ID = node.ID
		parsedNode.SourceURL = "manual"
		parsedNode.Status = node.Status
		parsedNode.CreatedAt = node.CreatedAt
		node = &parsedNode
	}
	if body.Name != nil {
		node.Name = *body.Name
	}
	if body.Region != nil {
		node.Region = *body.Region
	}
	node.UpdatedAt = time.Now().UTC()
	node.SourceURL = "manual"
	if err := s.store.NodeRepo().Save(r.Context(), node); err != nil {
		apiErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, toNodeResponse(node))
}

// POST /api/v1/nodes/{id}/enable
func (s *Server) enableNode(w http.ResponseWriter, r *http.Request) {
	if !s.requireStore(w) {
		return
	}
	id := r.PathValue("id")
	if err := s.store.NodeRepo().UpdateStatus(r.Context(), id, domain.NodeActive); err != nil {
		writeNotFoundOrError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// POST /api/v1/nodes/{id}/disable
func (s *Server) disableNode(w http.ResponseWriter, r *http.Request) {
	if !s.requireStore(w) {
		return
	}
	id := r.PathValue("id")
	if err := s.store.NodeRepo().UpdateStatus(r.Context(), id, domain.NodeDisabled); err != nil {
		writeNotFoundOrError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// DELETE /api/v1/nodes/{id}
func (s *Server) deleteNode(w http.ResponseWriter, r *http.Request) {
	if !s.requireStore(w) {
		return
	}
	id := r.PathValue("id")
	sources, err := s.store.NodeRepo().FindNodeSources(r.Context(), id)
	if err != nil {
		apiErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	if len(sources) > 0 {
		apiErr(w, http.StatusConflict, "subscription_node_managed", "subscription nodes are managed by their upstream")
		return
	}
	if err := s.store.NodeRepo().Delete(r.Context(), id); err != nil {
		writeNotFoundOrError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// GET /api/v1/nodes/{id}/measurements
func (s *Server) nodeMeasurements(w http.ResponseWriter, r *http.Request) {
	if !s.requireStore(w) {
		return
	}
	id := r.PathValue("id")
	limit := 50
	if v, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && v > 0 {
		limit = v
	}
	measurements, err := s.store.MeasurementRepo().FindByNodeID(r.Context(), id, limit)
	if err != nil {
		apiErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	resp := make([]measurementResponse, 0, len(measurements))
	for _, m := range measurements {
		resp = append(resp, toMeasurementResponse(m))
	}
	writeJSON(w, http.StatusOK, map[string]any{"measurements": resp})
}

func (s *Server) nodeCapability(w http.ResponseWriter, r *http.Request) {
	if !s.requireStore(w) {
		return
	}
	node, err := s.store.NodeRepo().FindByID(r.Context(), r.PathValue("id"))
	if err != nil {
		writeNotFoundOrError(w, err)
		return
	}
	available := false
	if value, ok := s.safeCoreStatus().(domain.CoreStatus); ok {
		available = value.Available
	}
	supported := map[string]bool{"http": true, "socks5": true, "socks": true, "ss": true, "ssr": true,
		"vmess": true, "vless": true, "trojan": true, "hysteria": true, "hysteria2": true, "tuic": true}
	known := supported[node.Protocol]
	result := map[string]any{"protocol": node.Protocol, "supported": available && known, "loadable": available && known, "testable": available && known}
	if !available {
		result["reason"] = domain.ErrCodeCoreUnavailable
	} else if !known {
		result["reason"] = "unsupported_protocol"
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) createNodeTest(w http.ResponseWriter, r *http.Request) {
	if !s.requireStore(w) {
		return
	}
	id := r.PathValue("id")
	if _, err := s.store.NodeRepo().FindByID(r.Context(), id); err != nil {
		writeNotFoundOrError(w, err)
		return
	}
	marker := strings.LastIndex(r.URL.Path, "/tests/")
	kind := "test_node"
	if marker >= 0 {
		kind = "test_" + strings.Trim(r.URL.Path[marker+len("/tests/"):], "/")
	}
	testKind := domain.TestKind(strings.TrimPrefix(kind, "test_"))
	if testKind != domain.TestConnectivity && testKind != domain.TestPing && testKind != domain.TestThroughput {
		apiErr(w, http.StatusBadRequest, "unsupported_test_kind", "supported tests are connectivity, ping and throughput")
		return
	}
	now := time.Now().UTC()
	if s.testSubmit != nil {
		jobID, err := s.testSubmit(r.Context(), id, testKind)
		if err != nil {
			if isTestQueueFull(err) {
				apiErr(w, http.StatusTooManyRequests, "test_queue_full", "test queue is full; retry later")
				return
			}
			var coded interface{ ErrorCode() string }
			if errors.As(err, &coded) && coded.ErrorCode() != "" {
				apiErr(w, http.StatusConflict, coded.ErrorCode(), err.Error())
				return
			}
			apiErr(w, http.StatusServiceUnavailable, "core_unavailable", err.Error())
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]string{"job_id": jobID})
		return
	}
	job := &domain.Job{ID: uuid.NewString(), Kind: kind, Status: domain.JobPending,
		EntityID: id, CreatedAt: now, UpdatedAt: now}
	if err := s.store.JobRepo().Save(r.Context(), job); err != nil {
		apiErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"job_id": job.ID})
}

// GET /api/v1/nodes/{id}/score
func (s *Server) nodeScore(w http.ResponseWriter, r *http.Request) {
	if !s.requireStore(w) {
		return
	}
	id := r.PathValue("id")
	snapshot, err := s.store.ScoreSnapshotRepo().FindLatestByNodeID(r.Context(), id)
	if err != nil {
		writeNotFoundOrError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toScoreSnapshotResponse(snapshot))
}

// GET /api/v1/nodes/{id}/score/events
func (s *Server) nodeScoreEvents(w http.ResponseWriter, r *http.Request) {
	if !s.requireStore(w) {
		return
	}
	id := r.PathValue("id")
	snapshots, err := s.store.ScoreSnapshotRepo().FindByNodeID(r.Context(), id, 100)
	if err != nil {
		apiErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	resp := make([]scoreSnapshotResponse, 0, len(snapshots))
	for i := range snapshots {
		resp = append(resp, toScoreSnapshotResponse(&snapshots[i]))
	}
	writeJSON(w, http.StatusOK, map[string]any{"events": resp})
}

// POST /api/v1/nodes/{id}/score/recalculate
func (s *Server) nodeScoreRecalculate(w http.ResponseWriter, r *http.Request) {
	if !s.requireStore(w) {
		return
	}
	id := r.PathValue("id")
	if s.scoreRecalculate != nil {
		jobID, err := s.scoreRecalculate(r.Context(), id)
		if err != nil {
			writeNotFoundOrError(w, err)
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]string{"job_id": jobID})
		return
	}
	now := time.Now().UTC()
	job := &domain.Job{
		ID:        uuid.NewString(),
		Kind:      "recalculate_score",
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

// enrichNodesWithScore loads and attaches the latest score snapshot for each node.
func enrichNodesWithScore(r *http.Request, s *Server, nodes []domain.Node) {
	for i := range nodes {
		snapshot, err := s.store.ScoreSnapshotRepo().FindLatestByNodeID(r.Context(), nodes[i].ID)
		if err == nil && snapshot != nil {
			score := snapshot.ToScore()
			nodes[i].Score = &score
		}
		if measurements, err := s.store.MeasurementRepo().FindByNodeID(r.Context(), nodes[i].ID, 20); err == nil {
			for _, measurement := range measurements {
				if measurement.Success && measurement.LatencyMS > 0 {
					value := measurement.LatencyMS
					nodes[i].LastLatencyMS = &value
					break
				}
			}
			for _, measurement := range measurements {
				if measurement.Success && measurement.SpeedBytesPerSec > 0 {
					value := measurement.SpeedBytesPerSec
					nodes[i].LastSpeedBPS = &value
					break
				}
			}
		}
	}
}
