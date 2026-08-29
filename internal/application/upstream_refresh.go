package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/teddymail/bagualu/internal/domain"
	"github.com/teddymail/bagualu/internal/modules/subscription_output"
)

const maxSubscriptionBody = 16 << 20

// UpstreamRefresher fetches and imports one subscription without deleting the
// previous node set when the remote source is unavailable or malformed.
type UpstreamRefresher struct {
	upstreams    domain.UpstreamRepository
	nodes        domain.NodeRepository
	measurements domain.MeasurementRepository
	client       *http.Client
}

// UpstreamRefreshRunner persists the asynchronous job around a refresher.
type UpstreamRefreshRunner struct {
	jobs      domain.JobRepository
	refresher *UpstreamRefresher
}

func NewUpstreamRefreshRunner(jobs domain.JobRepository, refresher *UpstreamRefresher) *UpstreamRefreshRunner {
	return &UpstreamRefreshRunner{jobs: jobs, refresher: refresher}
}

func (r *UpstreamRefreshRunner) Submit(ctx context.Context, upstreamID string) (string, error) {
	if r.jobs == nil || r.refresher == nil {
		return "", errors.New("upstream refresh is not configured")
	}
	now := time.Now().UTC()
	job := &domain.Job{ID: uuid.NewString(), Kind: "refresh_upstream", EntityID: upstreamID,
		Status: domain.JobPending, CreatedAt: now, UpdatedAt: now}
	if err := r.jobs.Save(ctx, job); err != nil {
		return "", err
	}
	go r.run(job.ID, upstreamID)
	return job.ID, nil
}

func (r *UpstreamRefreshRunner) run(jobID, upstreamID string) {
	ctx := context.Background()
	if err := r.jobs.UpdateStatus(ctx, jobID, domain.JobRunning, 10, ""); err != nil {
		return
	}
	_, err := r.refresher.Refresh(ctx, upstreamID)
	if err != nil {
		_ = r.jobs.UpdateStatus(ctx, jobID, domain.JobFailed, 100, err.Error())
		return
	}
	_ = r.jobs.UpdateStatus(ctx, jobID, domain.JobSucceeded, 100, "")
}

func NewUpstreamRefresher(upstreams domain.UpstreamRepository, nodes domain.NodeRepository, measurements domain.MeasurementRepository, client *http.Client) *UpstreamRefresher {
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}
	return &UpstreamRefresher{upstreams: upstreams, nodes: nodes, measurements: measurements, client: client}
}

// Refresh fetches, parses and imports the selected upstream. A RefreshRecord
// is written for both success and failure so the UI can explain stale data.
func (r *UpstreamRefresher) Refresh(ctx context.Context, upstreamID string) (domain.RefreshRecord, error) {
	record := domain.RefreshRecord{ID: uuid.NewString(), UpstreamID: upstreamID, CreatedAt: time.Now().UTC()}
	if r.upstreams == nil || r.nodes == nil {
		return r.saveFailure(ctx, record, errors.New("upstream repositories are not configured"))
	}
	upstream, err := r.upstreams.FindByID(ctx, upstreamID)
	if err != nil {
		return r.saveFailure(ctx, record, err)
	}
	body, err := r.fetch(ctx, upstream.URL)
	if err != nil {
		return r.saveFailure(ctx, record, err)
	}
	parsed, err := subscription_output.Parse(body, upstream.URL)
	if err != nil {
		return r.saveFailure(ctx, record, err)
	}

	now := time.Now().UTC()
	previousIDs := map[string]bool{}
	if index, ok := r.nodes.(interface {
		FindNodeIDsByUpstream(context.Context, string) ([]string, error)
	}); ok {
		if ids, indexErr := index.FindNodeIDsByUpstream(ctx, upstream.ID); indexErr == nil {
			for _, id := range ids {
				previousIDs[id] = true
			}
		}
	}
	seen := make(map[string]struct{}, len(parsed.Nodes))
	for i := range parsed.Nodes {
		node := parsed.Nodes[i]
		if _, ok := seen[node.ID]; ok {
			continue
		}
		if existing, findErr := r.nodes.FindByID(ctx, node.ID); findErr == nil {
			sources, sourceErr := r.nodes.FindNodeSources(ctx, node.ID)
			if sourceErr != nil {
				return r.saveFailure(ctx, record, fmt.Errorf("find node sources %s: %w", node.ID, sourceErr))
			}
			if len(sources) == 0 {
				if replacementID := r.findPreviousSubscriptionNode(ctx, upstream.ID, node, previousIDs); replacementID != "" {
					node.ID = replacementID
				} else {
					node.ID = uuid.NewString()
				}
			} else {
				node.CreatedAt = existing.CreatedAt
			}
		} else if replacementID := r.findPreviousSubscriptionNode(ctx, upstream.ID, node, previousIDs); replacementID != "" {
			node.ID = replacementID
		}
		seen[node.ID] = struct{}{}
		node.SourceURL = upstream.URL
		if node.EndpointIP == "" {
			node.EndpointIP = resolveEndpointIP(ctx, node.Address)
		}
		node.UpdatedAt = now
		if node.CreatedAt.IsZero() {
			node.CreatedAt = now
		}
		if existing, findErr := r.nodes.FindByID(ctx, node.ID); findErr == nil {
			// A manual disable is a management decision and must survive refresh.
			if existing.Status == domain.NodeDisabled {
				node.Status = domain.NodeDisabled
			}
			if node.CreatedAt.IsZero() {
				node.CreatedAt = existing.CreatedAt
			}
		}
		if err := r.nodes.Save(ctx, &node); err != nil {
			return r.saveFailure(ctx, record, fmt.Errorf("save node %s: %w", node.ID, err))
		}
		rawFragment, _ := json.Marshal(node.RawConfig)
		if err := r.nodes.SaveNodeSource(ctx, domain.NodeSource{
			NodeID: node.ID, UpstreamID: upstream.ID, OriginalName: node.Name,
			RawFragment: string(rawFragment), CreatedAt: now,
		}); err != nil {
			return r.saveFailure(ctx, record, fmt.Errorf("save node source %s: %w", node.ID, err))
		}
	}
	if index, ok := r.nodes.(interface {
		DeleteNodeSource(context.Context, string, string) error
	}); ok {
		for id := range previousIDs {
			if _, present := seen[id]; present {
				continue
			}
			if err := index.DeleteNodeSource(ctx, id, upstream.ID); err != nil {
				return r.saveFailure(ctx, record, err)
			}
			sources, sourceErr := r.nodes.FindNodeSources(ctx, id)
			if sourceErr == nil && len(sources) == 0 {
				if r.measurements != nil {
					if err := r.measurements.DeleteByNodeID(ctx, id); err != nil {
						return r.saveFailure(ctx, record, fmt.Errorf("delete measurements for node %s: %w", id, err))
					}
				}
				if node, findErr := r.nodes.FindByID(ctx, id); findErr == nil && node.Status != domain.NodeDisabled {
					_ = r.nodes.UpdateStatus(ctx, id, domain.NodeExpired)
				}
			}
		}
	}
	record.Success = true
	record.NodeCount = len(seen)
	if err := r.upstreams.SaveRefreshRecord(ctx, &record); err != nil {
		return record, err
	}
	return record, nil
}

func (r *UpstreamRefresher) findPreviousSubscriptionNode(ctx context.Context, upstreamID string, candidate domain.Node, previousIDs map[string]bool) string {
	for id := range previousIDs {
		node, err := r.nodes.FindByID(ctx, id)
		if err != nil || !sameNodeIdentity(*node, candidate) {
			continue
		}
		sources, err := r.nodes.FindNodeSources(ctx, id)
		if err != nil {
			continue
		}
		for _, source := range sources {
			if source.UpstreamID == upstreamID {
				return id
			}
		}
	}
	return ""
}

func sameNodeIdentity(left, right domain.Node) bool {
	if left.Protocol != right.Protocol || left.Address != right.Address || left.Port != right.Port {
		return false
	}
	leftConfig := cloneNodeConfig(left.RawConfig)
	rightConfig := cloneNodeConfig(right.RawConfig)
	delete(leftConfig, "name")
	delete(leftConfig, "uri")
	delete(rightConfig, "name")
	delete(rightConfig, "uri")
	leftRaw, leftErr := json.Marshal(leftConfig)
	rightRaw, rightErr := json.Marshal(rightConfig)
	return leftErr == nil && rightErr == nil && string(leftRaw) == string(rightRaw)
}

func cloneNodeConfig(config map[string]any) map[string]any {
	if config == nil {
		return map[string]any{}
	}
	clone := make(map[string]any, len(config))
	for key, value := range config {
		clone[key] = value
	}
	return clone
}

func resolveEndpointIP(ctx context.Context, address string) string {
	lookupCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	addresses, err := net.DefaultResolver.LookupIPAddr(lookupCtx, address)
	if err != nil || len(addresses) == 0 {
		return ""
	}
	return addresses[0].IP.String()
}

func (r *UpstreamRefresher) fetch(ctx context.Context, rawURL string) ([]byte, error) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return nil, fmt.Errorf("invalid upstream URL")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("build upstream request: %w", err)
	}
	req.Header.Set("Accept", "application/yaml, application/json, text/plain;q=0.9, */*;q=0.1")
	req.Header.Set("User-Agent", "Bagualu/1.0")
	resp, err := r.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch upstream: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("upstream returned HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxSubscriptionBody+1))
	if err != nil {
		return nil, fmt.Errorf("read upstream: %w", err)
	}
	if len(body) > maxSubscriptionBody {
		return nil, fmt.Errorf("upstream response exceeds %d bytes", maxSubscriptionBody)
	}
	return body, nil
}

func (r *UpstreamRefresher) saveFailure(ctx context.Context, record domain.RefreshRecord, err error) (domain.RefreshRecord, error) {
	record.Error = err.Error()
	if r.upstreams != nil {
		if saveErr := r.upstreams.SaveRefreshRecord(ctx, &record); saveErr != nil {
			return record, errors.Join(err, saveErr)
		}
	}
	return record, err
}
