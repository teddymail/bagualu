package application

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/teddymail/bagualu/internal/domain"
)

type ScoreService struct {
	nodes        domain.NodeRepository
	measurements domain.MeasurementRepository
	snapshots    domain.ScoreSnapshotRepository
	policy       domain.ScorePolicy
	mu           sync.RWMutex
}

func (s *ScoreService) Policy() domain.ScorePolicy {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.policy
}

func (s *ScoreService) SetPolicy(policy domain.ScorePolicy) error {
	if policy.Version < 1 || policy.Lookback <= 0 || policy.LatencyWindow < 1 || policy.SpeedWindow < 1 ||
		policy.MinimumAvailabilitySamples < 1 || policy.SpeedReference <= 0 ||
		policy.LatencyWeight < 0 || policy.SpeedWeight < 0 || policy.AvailabilityWeight < 0 ||
		policy.LatencyWeight+policy.SpeedWeight+policy.AvailabilityWeight <= 0 {
		return errors.New("invalid score policy")
	}
	s.mu.Lock()
	s.policy = policy
	s.mu.Unlock()
	return nil
}

func NewScoreService(nodes domain.NodeRepository, measurements domain.MeasurementRepository, snapshots domain.ScoreSnapshotRepository, policy domain.ScorePolicy) *ScoreService {
	if policy.Version == 0 {
		policy = domain.DefaultScorePolicy()
	}
	return &ScoreService{nodes: nodes, measurements: measurements, snapshots: snapshots, policy: policy}
}

func (s *ScoreService) Recalculate(ctx context.Context, nodeID string) (domain.ScoreSnapshot, error) {
	if s.nodes == nil || s.measurements == nil || s.snapshots == nil {
		return domain.ScoreSnapshot{}, errors.New("score service is not configured")
	}
	if _, err := s.nodes.FindByID(ctx, nodeID); err != nil {
		return domain.ScoreSnapshot{}, err
	}
	s.mu.RLock()
	policy := s.policy
	s.mu.RUnlock()
	now := time.Now().UTC()
	measurements, err := s.measurements.FindSince(ctx, nodeID, now.Add(-policy.Lookback))
	if err != nil {
		return domain.ScoreSnapshot{}, err
	}
	score := domain.CalculateScore(measurements, policy, now)
	for index := len(measurements) - 1; index >= 0; index-- {
		measurement := measurements[index]
		if measurement.Kind != string(domain.TestConnectivity) || measurement.Infrastructure {
			continue
		}
		if !measurement.Success && score.Status == domain.RecommendationRecommended {
			score.Status = domain.RecommendationTemporarilyUnavailable
		}
		break
	}
	snapshot := domain.ScoreSnapshotFromScore(uuid.NewString(), nodeID, score)
	if err := s.snapshots.Save(ctx, &snapshot); err != nil {
		return domain.ScoreSnapshot{}, err
	}
	return snapshot, nil
}

type ScoreRunner struct {
	jobs    domain.JobRepository
	service *ScoreService
}

func NewScoreRunner(jobs domain.JobRepository, service *ScoreService) *ScoreRunner {
	return &ScoreRunner{jobs: jobs, service: service}
}

func (r *ScoreRunner) Submit(ctx context.Context, nodeID string) (string, error) {
	if r.jobs == nil || r.service == nil {
		return "", errors.New("score recalculation is not configured")
	}
	now := time.Now().UTC()
	job := &domain.Job{ID: uuid.NewString(), Kind: "recalculate_score", EntityID: nodeID,
		Status: domain.JobPending, CreatedAt: now, UpdatedAt: now}
	if err := r.jobs.Save(ctx, job); err != nil {
		return "", err
	}
	go r.run(job.ID, nodeID)
	return job.ID, nil
}

func (r *ScoreRunner) run(jobID, nodeID string) {
	ctx := context.Background()
	if err := r.jobs.UpdateStatus(ctx, jobID, domain.JobRunning, 10, ""); err != nil {
		return
	}
	if _, err := r.service.Recalculate(ctx, nodeID); err != nil {
		_ = r.jobs.UpdateStatus(ctx, jobID, domain.JobFailed, 100, err.Error())
		return
	}
	_ = r.jobs.UpdateStatus(ctx, jobID, domain.JobSucceeded, 100, "")
}
