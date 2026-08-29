package mihomo

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestProcessManagerManualStopCancelsPendingRestart(t *testing.T) {
	directory := t.TempDir()
	countFile := filepath.Join(directory, "count")
	binary := filepath.Join(directory, "fake-mihomo")
	script := "#!/bin/sh\necho x >> " + countFile + "\nexit 1\n"
	if err := os.WriteFile(binary, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	manager := NewProcessManager(binary, filepath.Join(directory, "config.yaml"))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := manager.Start(ctx); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for {
		if _, err := os.Stat(countFile); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("fake Mihomo process did not start")
		}
		time.Sleep(10 * time.Millisecond)
	}
	manager.Monitor(ctx, 3, 250*time.Millisecond)
	if err := manager.Stop(); err != nil {
		t.Fatal(err)
	}
	time.Sleep(350 * time.Millisecond)
	data, err := os.ReadFile(countFile)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(data), "x") != 1 {
		t.Fatalf("manual stop allowed an automatic restart: %q", data)
	}
}
