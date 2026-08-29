package application

import (
	"testing"
	"time"

	"github.com/teddymail/bagualu/internal/domain"
)

func TestEndpointCircuitRequiresRepeatedNetworkFailures(t *testing.T) {
	circuit := NewEndpointCircuit(2, time.Minute)
	node := domain.Node{ID: "node-a", EndpointIP: "192.0.2.10"}
	failure := domain.MeasurementOutcome{ErrorCode: domain.ErrCodeNetworkUnreachable}
	if circuit.Record(node, failure) {
		t.Fatal("circuit opened after one failure")
	}
	if !circuit.Record(node, failure) {
		t.Fatal("circuit did not open after repeated network failures")
	}
	if err := circuit.Before(node); err == nil || err.Error() != domain.ErrCodeEndpointUnreachable {
		t.Fatalf("expected endpoint circuit block, got %v", err)
	}
	if circuit.Record(node, domain.MeasurementOutcome{Success: false, ErrorCode: domain.ErrCodeNodeLoadFailed}) {
		t.Fatal("protocol failure opened endpoint circuit")
	}
}

func TestEndpointCircuitAllowsRepresentativeRecovery(t *testing.T) {
	circuit := NewEndpointCircuit(2, time.Minute)
	circuit.now = func() time.Time { return time.Unix(100, 0) }
	node := domain.Node{ID: "node-a", EndpointIP: "192.0.2.11"}
	failure := domain.MeasurementOutcome{ErrorCode: domain.ErrCodeHostUnreachable}
	circuit.Record(node, failure)
	circuit.Record(node, failure)
	circuit.now = func() time.Time { return time.Unix(161, 0) }
	if err := circuit.Before(node); err != nil {
		t.Fatalf("representative recovery was blocked: %v", err)
	}
	if circuit.Record(node, domain.MeasurementOutcome{Success: true}) {
		t.Fatal("successful recovery kept circuit open")
	}
	if err := circuit.Before(node); err != nil {
		t.Fatalf("node remained blocked after recovery: %v", err)
	}
}
