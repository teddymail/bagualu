package application

import (
	"context"
	"sort"
	"time"

	"github.com/teddymail/bagualu/internal/domain"
)

type AutomaticTestPolicy struct {
	PingInterval            time.Duration
	ThroughputAllowed       bool
	ThroughputDayStart      time.Time
	ThroughputRetryInterval time.Duration
	SpeedSource             string
}

type AutomaticTestPlanner struct {
	nodes        domain.NodeRepository
	measurements domain.MeasurementRepository
	busy         func() bool
	submit       func(context.Context, string, domain.TestKind, string) (string, error)
}

func NewAutomaticTestPlanner(
	nodes domain.NodeRepository,
	measurements domain.MeasurementRepository,
	busy func() bool,
	submit func(context.Context, string, domain.TestKind, string) (string, error),
) *AutomaticTestPlanner {
	return &AutomaticTestPlanner{nodes: nodes, measurements: measurements, busy: busy, submit: submit}
}

func (p *AutomaticTestPlanner) Run(ctx context.Context, now time.Time, policy AutomaticTestPolicy) error {
	if p == nil || p.nodes == nil || p.measurements == nil || p.submit == nil || p.busy != nil && p.busy() {
		return nil
	}
	nodes, err := p.nodes.FindAll(ctx, domain.NodeFilter{})
	if err != nil {
		return err
	}
	if policy.ThroughputAllowed && policy.SpeedSource != "" {
		candidate, err := p.nextThroughput(ctx, nodes, now, policy)
		if err != nil {
			return err
		}
		if candidate != nil {
			_, err = p.submit(ctx, candidate.ID, domain.TestThroughput, policy.SpeedSource)
			return err
		}
	}
	return p.schedulePing(ctx, nodes, now, policy.PingInterval)
}

func (p *AutomaticTestPlanner) nextThroughput(ctx context.Context, nodes []domain.Node, now time.Time, policy AutomaticTestPolicy) (*domain.Node, error) {
	latestAttempts, err := p.measurements.FindLatestTimesByKind(ctx, string(domain.TestThroughput), false)
	if err != nil {
		return nil, err
	}
	latestNodeResults, err := p.measurements.FindLatestTimesByKind(ctx, string(domain.TestThroughput), true)
	if err != nil {
		return nil, err
	}
	retryInterval := policy.ThroughputRetryInterval
	if retryInterval <= 0 {
		retryInterval = 5 * time.Minute
	}
	candidates := make([]testCandidate, 0, len(nodes))
	for i := range nodes {
		node := &nodes[i]
		lastResult, hasResult := latestNodeResults[node.ID]
		if node.Status != domain.NodeActive || hasResult && !lastResult.Before(policy.ThroughputDayStart) {
			continue
		}
		lastAttempt := latestAttempts[node.ID]
		if !lastAttempt.IsZero() && lastAttempt.Add(retryInterval).After(now) {
			continue
		}
		candidates = append(candidates, testCandidate{node: node, last: lastAttempt})
	}
	sortCandidates(candidates)
	if len(candidates) == 0 {
		return nil, nil
	}
	return candidates[0].node, nil
}

func (p *AutomaticTestPlanner) schedulePing(ctx context.Context, nodes []domain.Node, now time.Time, interval time.Duration) error {
	if interval <= 0 {
		interval = time.Minute
	}
	pingTimes, err := p.measurements.FindLatestTimesByKind(ctx, string(domain.TestPing), false)
	if err != nil {
		return err
	}
	connectivityTimes, err := p.measurements.FindLatestTimesByKind(ctx, string(domain.TestConnectivity), false)
	if err != nil {
		return err
	}
	candidates := make([]testCandidate, 0, len(nodes))
	for i := range nodes {
		node := &nodes[i]
		kind := domain.TestPing
		latest := pingTimes[node.ID]
		if node.Status == domain.NodeEndpointUnreachable {
			kind = domain.TestConnectivity
			latest = connectivityTimes[node.ID]
		} else if node.Status != domain.NodeActive {
			continue
		}
		if !latest.IsZero() && latest.Add(interval).After(now) {
			continue
		}
		candidates = append(candidates, testCandidate{node: node, kind: kind, last: latest})
	}
	sortCandidates(candidates)
	for _, candidate := range candidates {
		if _, err := p.submit(ctx, candidate.node.ID, candidate.kind, ""); err != nil {
			if candidate.kind == domain.TestConnectivity {
				continue
			}
			return err
		}
		return nil
	}
	return nil
}

type testCandidate struct {
	node *domain.Node
	kind domain.TestKind
	last time.Time
}

func sortCandidates(candidates []testCandidate) {
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].last.Equal(candidates[j].last) {
			return candidates[i].node.ID < candidates[j].node.ID
		}
		if candidates[i].last.IsZero() {
			return true
		}
		if candidates[j].last.IsZero() {
			return false
		}
		return candidates[i].last.Before(candidates[j].last)
	})
}
