package domain

import (
	"testing"
	"time"
)

func TestCalculateScoreUsesHarmonicMean(t *testing.T) {
	now := time.Now()
	measurements := make([]Measurement, 0, 22)
	for i := 0; i < 20; i++ {
		measurements = append(measurements, Measurement{Kind: "connectivity", Success: i != 19, LatencyMS: 50, CreatedAt: now})
	}
	measurements = append(measurements,
		Measurement{Kind: "throughput", Success: true, SpeedBytesPerSec: 1_000_000, CreatedAt: now},
		Measurement{Kind: "throughput", Success: true, SpeedBytesPerSec: 1_000_000, CreatedAt: now},
		Measurement{Kind: "throughput", Success: true, SpeedBytesPerSec: 1_000_000, CreatedAt: now},
	)
	score := CalculateScore(measurements, DefaultScorePolicy(), now)
	if score.Status != RecommendationRecommended || score.Overall != 98 {
		t.Fatalf("unexpected score: %+v", score)
	}
}

func TestInfrastructureFailureDoesNotAffectAvailability(t *testing.T) {
	now := time.Now()
	policy := DefaultScorePolicy()
	measurements := make([]Measurement, 0, 23)
	for i := 0; i < 20; i++ {
		measurements = append(measurements, Measurement{Kind: "connectivity", Success: true, LatencyMS: 100, CreatedAt: now})
	}
	measurements = append(measurements, Measurement{Kind: "connectivity", Infrastructure: true, CreatedAt: now})
	score := CalculateScore(measurements, policy, now)
	if score.Availability != 100 || score.AvailabilitySamples != 20 {
		t.Fatalf("infrastructure sample included: %+v", score)
	}
}
