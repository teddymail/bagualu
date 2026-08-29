package domain

import "time"

// ScoreSnapshot records a point-in-time snapshot of a node's computed score.
// Snapshots are immutable once saved and support full score replay from raw measurements.
type ScoreSnapshot struct {
	ID                  string
	NodeID              string
	Latency             float64
	Speed               float64
	Availability        float64
	Overall             float64
	Status              Recommendation
	LatencySamples      int
	SpeedSamples        int
	AvailabilitySamples int
	StrategyVersion     int
	CalculatedAt        time.Time
}

// ScoreSnapshotFromScore creates a ScoreSnapshot from a computed Score.
func ScoreSnapshotFromScore(id, nodeID string, s Score) ScoreSnapshot {
	return ScoreSnapshot{
		ID:                  id,
		NodeID:              nodeID,
		Latency:             s.Latency,
		Speed:               s.Speed,
		Availability:        s.Availability,
		Overall:             s.Overall,
		Status:              s.Status,
		LatencySamples:      s.LatencySamples,
		SpeedSamples:        s.SpeedSamples,
		AvailabilitySamples: s.AvailabilitySamples,
		StrategyVersion:     s.StrategyVersion,
		CalculatedAt:        s.CalculatedAt,
	}
}

// ToScore converts the snapshot back to a Score value for replay or comparison.
func (s ScoreSnapshot) ToScore() Score {
	return Score{
		Latency:             s.Latency,
		Speed:               s.Speed,
		Availability:        s.Availability,
		Overall:             s.Overall,
		Status:              s.Status,
		LatencySamples:      s.LatencySamples,
		SpeedSamples:        s.SpeedSamples,
		AvailabilitySamples: s.AvailabilitySamples,
		StrategyVersion:     s.StrategyVersion,
		CalculatedAt:        s.CalculatedAt,
	}
}
