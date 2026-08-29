package application

import (
	"fmt"
	"sync"
	"time"

	"github.com/teddymail/bagualu/internal/domain"
)

type EndpointCircuit struct {
	mu           sync.Mutex
	entries      map[string]*endpointCircuitEntry
	failureLimit int
	cooldown     time.Duration
	now          func() time.Time
}

type endpointCircuitEntry struct {
	failures       int
	openUntil      time.Time
	representative string
	probeInFlight  bool
}

type EndpointCircuitError struct {
	RetryAt time.Time
}

func (e *EndpointCircuitError) Error() string     { return domain.ErrCodeEndpointUnreachable }
func (e *EndpointCircuitError) ErrorCode() string { return domain.ErrCodeEndpointUnreachable }

func NewEndpointCircuit(failureLimit int, cooldown time.Duration) *EndpointCircuit {
	if failureLimit < 2 {
		failureLimit = 2
	}
	if cooldown <= 0 {
		cooldown = 5 * time.Minute
	}
	return &EndpointCircuit{entries: make(map[string]*endpointCircuitEntry), failureLimit: failureLimit, cooldown: cooldown, now: time.Now}
}

func (c *EndpointCircuit) Before(node domain.Node) error {
	if c == nil || node.EndpointIP == "" {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	entry := c.entries[node.EndpointIP]
	if entry == nil || entry.openUntil.IsZero() {
		return nil
	}
	now := c.now()
	if now.Before(entry.openUntil) {
		return &EndpointCircuitError{RetryAt: entry.openUntil}
	}
	if entry.probeInFlight || node.ID != entry.representative {
		return &EndpointCircuitError{RetryAt: now}
	}
	entry.probeInFlight = true
	return nil
}

func (c *EndpointCircuit) Record(node domain.Node, outcome domain.MeasurementOutcome) bool {
	if c == nil || node.EndpointIP == "" || outcome.Infrastructure {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	entry := c.entries[node.EndpointIP]
	if entry == nil {
		entry = &endpointCircuitEntry{}
		c.entries[node.EndpointIP] = entry
	}
	if outcome.Success {
		entry.failures = 0
		entry.openUntil = time.Time{}
		entry.probeInFlight = false
		return false
	}
	if outcome.ErrorCode != domain.ErrCodeNetworkUnreachable && outcome.ErrorCode != domain.ErrCodeHostUnreachable {
		return false
	}
	entry.failures++
	if entry.failures < c.failureLimit {
		return false
	}
	entry.representative = node.ID
	entry.openUntil = c.now().Add(c.cooldown)
	entry.probeInFlight = false
	return true
}

func (c *EndpointCircuit) String() string {
	if c == nil {
		return ""
	}
	return fmt.Sprintf("endpoint circuit (%d failures)", c.failureLimit)
}
