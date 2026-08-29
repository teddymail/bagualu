package network

import "testing"

func TestParsePingOutput(t *testing.T) {
	result := ParsePingOutput("64 bytes from 192.0.2.1: time=12.5 ms\n64 bytes from 192.0.2.1: time<1 ms\n", 3)
	if result.Received != 2 || result.LatencyMS != 6.75 || result.Probes != 3 {
		t.Fatalf("unexpected ping result: %+v", result)
	}
}
