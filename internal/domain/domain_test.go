package domain

import (
	"testing"
	"time"
)

func TestAPIKeyIsActive(t *testing.T) {
	now := time.Now().UTC()
	past := now.Add(-time.Hour)
	future := now.Add(time.Hour)

	cases := []struct {
		name   string
		k      APIKey
		active bool
	}{
		{"no expiry no revoke", APIKey{}, true},
		{"expired", APIKey{ExpiresAt: &past}, false},
		{"not yet expired", APIKey{ExpiresAt: &future}, true},
		{"revoked", APIKey{RevokedAt: &past}, false},
		{"revoked and not expired", APIKey{RevokedAt: &past, ExpiresAt: &future}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.k.IsActive(now); got != tc.active {
				t.Errorf("IsActive() = %v, want %v", got, tc.active)
			}
		})
	}
}

func TestSubscriptionLinkIsAccessible(t *testing.T) {
	now := time.Now().UTC()
	past := now.Add(-time.Hour)
	future := now.Add(time.Hour)

	cases := []struct {
		name       string
		l          SubscriptionLink
		accessible bool
	}{
		{"enabled no expiry", SubscriptionLink{Enabled: true}, true},
		{"disabled", SubscriptionLink{Enabled: false}, false},
		{"enabled expired", SubscriptionLink{Enabled: true, ExpiresAt: &past}, false},
		{"enabled not expired", SubscriptionLink{Enabled: true, ExpiresAt: &future}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.l.IsAccessible(now); got != tc.accessible {
				t.Errorf("IsAccessible() = %v, want %v", got, tc.accessible)
			}
		})
	}
}

func TestJobIsTerminal(t *testing.T) {
	terminal := []JobStatus{JobDone, JobFailed, JobCancelled}
	for _, s := range terminal {
		j := Job{Status: s}
		if !j.IsTerminal() {
			t.Errorf("expected %q to be terminal", s)
		}
	}
	nonTerminal := []JobStatus{JobPending, JobRunning}
	for _, s := range nonTerminal {
		j := Job{Status: s}
		if j.IsTerminal() {
			t.Errorf("expected %q to be non-terminal", s)
		}
	}
}

func TestScoreSnapshotRoundTrip(t *testing.T) {
	s := Score{
		Latency: 80, Speed: 70, Availability: 90, Overall: 78,
		Status:         RecommendationRecommended,
		LatencySamples: 10, SpeedSamples: 3, AvailabilitySamples: 20,
		StrategyVersion: 1,
		CalculatedAt:    time.Now().UTC().Truncate(time.Second),
	}
	snap := ScoreSnapshotFromScore("snap-1", "node-1", s)
	if snap.NodeID != "node-1" || snap.ID != "snap-1" {
		t.Fatalf("snapshot IDs wrong: %+v", snap)
	}
	back := snap.ToScore()
	if back.Latency != s.Latency || back.Overall != s.Overall || back.Status != s.Status {
		t.Fatalf("round-trip mismatch: got %+v, want %+v", back, s)
	}
	if !back.CalculatedAt.Equal(s.CalculatedAt) {
		t.Fatalf("time mismatch: %v vs %v", back.CalculatedAt, s.CalculatedAt)
	}
}
