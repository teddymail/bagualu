package application

import (
	"context"
	"testing"
	"time"

	"github.com/teddymail/bagualu/internal/domain"
	"github.com/teddymail/bagualu/internal/infrastructure/persistence"
)

func TestScoreServiceRecalculatesAndPersistsSnapshot(t *testing.T) {
	store, err := persistence.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	now := time.Now().UTC()
	node := &domain.Node{ID: "score-node", Name: "node", Protocol: "socks5", Address: "192.0.2.1", Port: 1080, Status: domain.NodeActive, CreatedAt: now, UpdatedAt: now}
	if err := store.NodeRepo().Save(context.Background(), node); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 20; i++ {
		if err := store.MeasurementRepo().Save(context.Background(), &domain.Measurement{ID: "m-" + string(rune('a'+i)), NodeID: node.ID, Kind: "connectivity", Success: true, LatencyMS: 50, CreatedAt: now}); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 3; i++ {
		if err := store.MeasurementRepo().Save(context.Background(), &domain.Measurement{ID: "s-" + string(rune('a'+i)), NodeID: node.ID, Kind: "throughput", Success: true, SpeedBytesPerSec: 900000, CreatedAt: now}); err != nil {
			t.Fatal(err)
		}
	}
	service := NewScoreService(store.NodeRepo(), store.MeasurementRepo(), store.ScoreSnapshotRepo(), domain.DefaultScorePolicy())
	snapshot, err := service.Recalculate(context.Background(), node.ID)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.AvailabilitySamples != 20 || snapshot.SpeedSamples != 3 || snapshot.Status != domain.RecommendationRecommended {
		t.Fatalf("unexpected score snapshot: %+v", snapshot)
	}
	latest, err := store.ScoreSnapshotRepo().FindLatestByNodeID(context.Background(), node.ID)
	if err != nil || latest.ID != snapshot.ID {
		t.Fatalf("snapshot was not persisted: %v %+v", err, latest)
	}
}

func TestScoreServiceMarksRecommendedNodeTemporarilyUnavailableAfterLatestFailure(t *testing.T) {
	store, err := persistence.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	now := time.Now().UTC()
	node := &domain.Node{ID: "score-failure", Name: "node", Protocol: "socks5", Address: "192.0.2.2", Port: 1080, Status: domain.NodeActive, CreatedAt: now, UpdatedAt: now}
	if err := store.NodeRepo().Save(context.Background(), node); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 20; i++ {
		if err := store.MeasurementRepo().Save(context.Background(), &domain.Measurement{ID: "c-" + string(rune('a'+i)), NodeID: node.ID, Kind: "connectivity", Success: true, LatencyMS: 50, CreatedAt: now}); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 3; i++ {
		if err := store.MeasurementRepo().Save(context.Background(), &domain.Measurement{ID: "t-" + string(rune('a'+i)), NodeID: node.ID, Kind: "throughput", Success: true, SpeedBytesPerSec: 900000, CreatedAt: now}); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.MeasurementRepo().Save(context.Background(), &domain.Measurement{ID: "latest-failure", NodeID: node.ID, Kind: "connectivity", Success: false, CreatedAt: now.Add(time.Second)}); err != nil {
		t.Fatal(err)
	}
	snapshot, err := NewScoreService(store.NodeRepo(), store.MeasurementRepo(), store.ScoreSnapshotRepo(), domain.DefaultScorePolicy()).Recalculate(context.Background(), node.ID)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Status != domain.RecommendationTemporarilyUnavailable {
		t.Fatalf("expected temporarily unavailable, got %+v", snapshot)
	}
}
