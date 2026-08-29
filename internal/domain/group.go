package domain

import "time"

// Group is a named collection of nodes used for resource sharing or access control.
type Group struct {
	ID               string
	Name             string
	Description      string
	MinScore         float64 // resource output threshold; application layer enforces >= 60
	OnePerEndpointIP bool    // when true, only the highest-scoring node per endpoint IP is exported
	CreatedAt        time.Time
	UpdatedAt        time.Time
}
