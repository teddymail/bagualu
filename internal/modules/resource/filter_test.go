package resource

import (
	"github.com/teddymail/bagualu/internal/domain"
	"testing"
)

func TestSelectNeverLowersMinimumScore(t *testing.T) {
	nodes := []domain.Node{
		{ID: "good", EndpointIP: "1.1.1.1", Status: domain.NodeActive, Score: &domain.Score{Overall: 80, Status: domain.RecommendationRecommended}},
		{ID: "low", EndpointIP: "2.2.2.2", Status: domain.NodeActive, Score: &domain.Score{Overall: 59, Status: domain.RecommendationNotRecommended}},
	}

	got := Select(nodes, Filter{MinScore: 0, OnePerEndpointIP: true})
	if len(got) != 1 || got[0].ID != "good" {
		t.Fatalf("unexpected selection: %+v", got)
	}
}

func TestSelectHidesUnavailableNodesAndKeepsBestEndpoint(t *testing.T) {
	nodes := []domain.Node{
		{ID: "best", EndpointIP: "1.1.1.1", Status: domain.NodeActive, Score: &domain.Score{Overall: 90, Availability: 95, Latency: 20, Speed: 80, Status: domain.RecommendationRecommended}},
		{ID: "lower", EndpointIP: "1.1.1.1", Status: domain.NodeActive, Score: &domain.Score{Overall: 80, Availability: 95, Latency: 20, Speed: 80, Status: domain.RecommendationRecommended}},
		{ID: "unreachable", EndpointIP: "2.2.2.2", Status: domain.NodeUnreachable, Score: &domain.Score{Overall: 95, Status: domain.RecommendationRecommended}},
	}
	got := Select(nodes, Filter{OnePerEndpointIP: true})
	if len(got) != 1 || got[0].ID != "best" {
		t.Fatalf("unexpected selection: %+v", got)
	}
}
