package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestReadUCIConfigPreservesQuotedValues(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bagualu")
	content := "config bagualu 'main'\noption listen '127.0.0.1'\noption throughput_windows '02:00-06:00, 23:00-23:30' # local windows\n"
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	values, err := readUCIConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if values["throughput_windows"] != "02:00-06:00, 23:00-23:30" {
		t.Fatalf("quoted value was truncated: %q", values["throughput_windows"])
	}
}

func TestSelectSpeedSourceIsStablePerDay(t *testing.T) {
	policy := runtimeTestPolicy{ThroughputURLs: []string{"a", "b", "c"}}
	first := time.Date(2026, 8, 29, 1, 0, 0, 0, time.Local)
	if selectSpeedSource(policy, first) != selectSpeedSource(policy, first.Add(12*time.Hour)) {
		t.Fatal("same daily batch selected different sources")
	}
	if selectSpeedSource(policy, first) == selectSpeedSource(policy, first.Add(24*time.Hour)) {
		t.Fatal("consecutive daily batches did not rotate sources")
	}
}

func TestWriteRuntimeStatus(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runtime", "status.json")
	if err := writeRuntimeStatus(path, map[string]any{"service_state": "running"}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != `{"service_state":"running"}` {
		t.Fatalf("unexpected status payload: %s", data)
	}
}
