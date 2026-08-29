package application_test

import (
	"context"
	"testing"
	"time"

	"github.com/teddymail/bagualu/internal/application"
	"github.com/teddymail/bagualu/internal/domain"
	"github.com/teddymail/bagualu/internal/infrastructure/persistence"
)

func TestAutomaticTestPlannerSchedulesOldestDuePingOnly(t *testing.T) {
	store, err := persistence.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	now := time.Date(2026, 8, 30, 1, 0, 0, 0, time.UTC)
	for _, node := range []*domain.Node{
		{ID: "newer", Name: "newer", Status: domain.NodeActive, CreatedAt: now, UpdatedAt: now},
		{ID: "older", Name: "older", Status: domain.NodeActive, CreatedAt: now, UpdatedAt: now},
	} {
		if err := store.NodeRepo().Save(context.Background(), node); err != nil {
			t.Fatal(err)
		}
	}
	for _, measurement := range []*domain.Measurement{
		{ID: "ping-newer", NodeID: "newer", Kind: string(domain.TestPing), CreatedAt: now.Add(-2 * time.Minute)},
		{ID: "ping-older", NodeID: "older", Kind: string(domain.TestPing), CreatedAt: now.Add(-10 * time.Minute)},
	} {
		if err := store.MeasurementRepo().Save(context.Background(), measurement); err != nil {
			t.Fatal(err)
		}
	}
	var submitted []string
	planner := application.NewAutomaticTestPlanner(store.NodeRepo(), store.MeasurementRepo(), nil,
		func(_ context.Context, nodeID string, kind domain.TestKind, _ string) (string, error) {
			submitted = append(submitted, nodeID+":"+string(kind))
			return "job", nil
		})
	if err := planner.Run(context.Background(), now, application.AutomaticTestPolicy{PingInterval: time.Minute}); err != nil {
		t.Fatal(err)
	}
	if len(submitted) != 1 || submitted[0] != "older:ping" {
		t.Fatalf("unexpected submissions: %v", submitted)
	}
}

func TestAutomaticTestPlannerPrioritizesDailyThroughputAndCoolsInfrastructureRetry(t *testing.T) {
	store, err := persistence.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	now := time.Date(2026, 8, 30, 3, 0, 0, 0, time.UTC)
	for _, node := range []*domain.Node{
		{ID: "recent-infrastructure", Name: "recent", Status: domain.NodeActive, CreatedAt: now, UpdatedAt: now},
		{ID: "untested", Name: "untested", Status: domain.NodeActive, CreatedAt: now, UpdatedAt: now},
	} {
		if err := store.NodeRepo().Save(context.Background(), node); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.MeasurementRepo().Save(context.Background(), &domain.Measurement{
		ID: "infra", NodeID: "recent-infrastructure", Kind: string(domain.TestThroughput), Infrastructure: true, CreatedAt: now.Add(-time.Minute),
	}); err != nil {
		t.Fatal(err)
	}
	var nodeID string
	var kind domain.TestKind
	planner := application.NewAutomaticTestPlanner(store.NodeRepo(), store.MeasurementRepo(), nil,
		func(_ context.Context, submittedNodeID string, submittedKind domain.TestKind, _ string) (string, error) {
			nodeID, kind = submittedNodeID, submittedKind
			return "job", nil
		})
	policy := application.AutomaticTestPolicy{
		PingInterval:            time.Minute,
		ThroughputAllowed:       true,
		ThroughputDayStart:      time.Date(2026, 8, 30, 0, 0, 0, 0, time.UTC),
		ThroughputRetryInterval: 5 * time.Minute,
		SpeedSource:             "https://speed.test/file",
	}
	if err := planner.Run(context.Background(), now, policy); err != nil {
		t.Fatal(err)
	}
	if nodeID != "untested" || kind != domain.TestThroughput {
		t.Fatalf("scheduled node=%s kind=%s", nodeID, kind)
	}
}

func TestAutomaticTestPlannerDoesNothingWhileQueueBusy(t *testing.T) {
	store, err := persistence.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	now := time.Now().UTC()
	if err := store.NodeRepo().Save(context.Background(), &domain.Node{ID: "node", Name: "node", Status: domain.NodeActive, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	called := false
	planner := application.NewAutomaticTestPlanner(store.NodeRepo(), store.MeasurementRepo(), func() bool { return true },
		func(context.Context, string, domain.TestKind, string) (string, error) {
			called = true
			return "job", nil
		})
	if err := planner.Run(context.Background(), now, application.AutomaticTestPolicy{PingInterval: time.Minute}); err != nil {
		t.Fatal(err)
	}
	if called {
		t.Fatal("busy queue accepted an automatic task")
	}
}
