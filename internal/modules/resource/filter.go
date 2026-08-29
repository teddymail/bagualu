package resource

import (
	"sort"
	"strings"

	"github.com/teddymail/bagualu/internal/domain"
)

type Filter struct {
	Protocol, Region, Tag, Sort string
	MinScore                    float64
	Limit                       int
	OnePerEndpointIP            bool
}

func Select(nodes []domain.Node, filter Filter) []domain.Node {
	selected, _ := SelectWithReasons(nodes, filter)
	return selected
}

type SkipReason struct {
	NodeID string
	Reason string
}

func SelectWithReasons(nodes []domain.Node, filter Filter) ([]domain.Node, []SkipReason) {
	if filter.MinScore < 60 {
		filter.MinScore = 60
	}
	selected := make([]domain.Node, 0, len(nodes))
	skipped := make([]SkipReason, 0)
	for _, node := range nodes {
		if node.Status != domain.NodeActive {
			skipped = append(skipped, SkipReason{node.ID, "status_" + string(node.Status)})
			continue
		}
		if filter.Protocol != "" && node.Protocol != filter.Protocol || filter.Region != "" && node.Region != filter.Region {
			skipped = append(skipped, SkipReason{node.ID, "filter_mismatch"})
			continue
		}
		if filter.Tag != "" && !strings.Contains(strings.ToLower(node.Name), strings.ToLower(filter.Tag)) {
			skipped = append(skipped, SkipReason{node.ID, "tag_mismatch"})
			continue
		}
		if node.Score == nil || node.Score.Status != domain.RecommendationRecommended || node.Score.Overall < filter.MinScore {
			skipped = append(skipped, SkipReason{node.ID, "score_below_minimum_or_unrated"})
			continue
		}
		selected = append(selected, node)
	}
	sort.SliceStable(selected, func(i, j int) bool {
		if filter.Sort == "name" {
			return selected[i].Name < selected[j].Name
		}
		if filter.Sort == "latency" && selected[i].Score.Latency != selected[j].Score.Latency {
			return selected[i].Score.Latency < selected[j].Score.Latency
		}
		if filter.Sort == "speed" && selected[i].Score.Speed != selected[j].Score.Speed {
			return selected[i].Score.Speed > selected[j].Score.Speed
		}
		if selected[i].Score.Overall != selected[j].Score.Overall {
			return selected[i].Score.Overall > selected[j].Score.Overall
		}
		if selected[i].Score.Availability != selected[j].Score.Availability {
			return selected[i].Score.Availability > selected[j].Score.Availability
		}
		if selected[i].Score.Latency != selected[j].Score.Latency {
			return selected[i].Score.Latency < selected[j].Score.Latency
		}
		if selected[i].Score.Speed != selected[j].Score.Speed {
			return selected[i].Score.Speed > selected[j].Score.Speed
		}
		return selected[i].ID < selected[j].ID
	})
	if filter.OnePerEndpointIP {
		seen := map[string]bool{}
		unique := selected[:0]
		for _, node := range selected {
			key := node.EndpointIP
			if key != "" && seen[key] {
				skipped = append(skipped, SkipReason{node.ID, "duplicate_endpoint_ip"})
				continue
			}
			if key != "" {
				seen[key] = true
			}
			unique = append(unique, node)
		}
		selected = unique
	}
	if filter.Limit > 0 && len(selected) > filter.Limit {
		for _, node := range selected[filter.Limit:] {
			skipped = append(skipped, SkipReason{node.ID, "node_limit"})
		}
		selected = selected[:filter.Limit]
	}
	return selected, skipped
}
