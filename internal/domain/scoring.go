package domain

import (
	"math"
	"sort"
	"time"
)

type ScorePolicy struct {
	LatencyWindow              int           `json:"latency_window"`
	SpeedWindow                int           `json:"speed_window"`
	AvailabilityWindow         int           `json:"availability_window"`
	MinimumAvailabilitySamples int           `json:"minimum_availability_samples"`
	Lookback                   time.Duration `json:"lookback"`
	SpeedReference             float64       `json:"speed_reference"`
	LatencyWeight              float64       `json:"latency_weight"`
	SpeedWeight                float64       `json:"speed_weight"`
	AvailabilityWeight         float64       `json:"availability_weight"`
	RecommendationThreshold    float64       `json:"recommendation_threshold"`
	Version                    int           `json:"version"`
}

func DefaultScorePolicy() ScorePolicy {
	return ScorePolicy{20, 3, 100, 20, 7 * 24 * time.Hour, 1_000_000, .30, .30, .40, 60, 1}
}

func Median(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	copyValues := append([]float64(nil), values...)
	sort.Float64s(copyValues)
	middle := len(copyValues) / 2
	if len(copyValues)%2 == 1 {
		return copyValues[middle]
	}
	return (copyValues[middle-1] + copyValues[middle]) / 2
}

func clamp(value, low, high float64) float64 {
	return math.Max(low, math.Min(high, value))
}

// CalculateScore is deterministic: infrastructure failures are excluded before this function.
func CalculateScore(measurements []Measurement, policy ScorePolicy, now time.Time) Score {
	cutoff := now.Add(-policy.Lookback)
	latencies, speeds := []float64{}, []float64{}
	successes, validAvailability := 0, 0
	for _, measurement := range measurements {
		if measurement.CreatedAt.Before(cutoff) || measurement.Infrastructure {
			continue
		}
		switch measurement.Kind {
		case "connectivity":
			validAvailability++
			if measurement.Success {
				successes++
			}
			if measurement.Success && measurement.LatencyMS > 0 {
				latencies = append(latencies, measurement.LatencyMS)
			}
		case "throughput":
			if measurement.Success && measurement.SpeedBytesPerSec > 0 {
				speeds = append(speeds, measurement.SpeedBytesPerSec)
			}
		}
	}
	if len(latencies) > policy.LatencyWindow {
		latencies = latencies[len(latencies)-policy.LatencyWindow:]
	}
	if len(speeds) > policy.SpeedWindow {
		speeds = speeds[len(speeds)-policy.SpeedWindow:]
	}
	result := Score{LatencySamples: len(latencies), SpeedSamples: len(speeds), AvailabilitySamples: validAvailability, StrategyVersion: policy.Version, CalculatedAt: now}
	if len(latencies) > 0 {
		result.Latency = clamp(100*(500-Median(latencies))/450, 0, 100)
	}
	if len(speeds) > 0 && policy.SpeedReference > 0 {
		result.Speed = clamp(100*Median(speeds)/policy.SpeedReference, 0, 100)
	}
	if validAvailability >= policy.MinimumAvailabilitySamples {
		result.Availability = 100 * float64(successes) / float64(validAvailability)
	}
	if len(latencies) > 0 && len(speeds) > 0 && validAvailability >= policy.MinimumAvailabilitySamples {
		if result.Latency == 0 || result.Speed == 0 || result.Availability == 0 {
			result.Overall = 0
		} else {
			result.Overall = 1 / (policy.LatencyWeight/result.Latency + policy.SpeedWeight/result.Speed + policy.AvailabilityWeight/result.Availability)
		}
		result.Overall = math.Round(clamp(result.Overall, 0, 100))
		result.Status = RecommendationNotRecommended
		if result.Overall >= policy.RecommendationThreshold {
			result.Status = RecommendationRecommended
		}
	} else {
		result.Status = RecommendationUnrated
	}
	return result
}
