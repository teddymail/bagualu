package application

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/teddymail/bagualu/internal/domain"
	"github.com/teddymail/bagualu/internal/infrastructure/persistence"
	"github.com/teddymail/bagualu/internal/modules/subscription_output"
)

func TestUpstreamRefresherImportsNodesAndMergesDuplicateIdentity(t *testing.T) {
	store, err := persistence.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/yaml")
		_, _ = w.Write([]byte("proxies:\n  - name: first\n    type: socks5\n    server: 192.0.2.10\n    port: 1080\n  - name: second\n    type: socks5\n    server: 192.0.2.10\n    port: 1080\n"))
	}))
	defer server.Close()

	now := time.Now().UTC()
	upstream := &domain.Upstream{ID: "upstream-1", Name: "test", URL: server.URL,
		Format: domain.UpstreamFormatRaw, Enabled: true, CreatedAt: now, UpdatedAt: now}
	if err := store.UpstreamRepo().Save(context.Background(), upstream); err != nil {
		t.Fatal(err)
	}

	refresher := NewUpstreamRefresher(store.UpstreamRepo(), store.NodeRepo(), store.MeasurementRepo(), server.Client())
	record, err := refresher.Refresh(context.Background(), upstream.ID)
	if err != nil {
		t.Fatalf("Refresh() error = %v", err)
	}
	if record.NodeCount != 1 || !record.Success {
		t.Fatalf("unexpected refresh record: %+v", record)
	}
	nodes, err := store.NodeRepo().FindAll(context.Background(), domain.NodeFilter{})
	if err != nil || len(nodes) != 1 {
		t.Fatalf("expected one merged node, got %d, err=%v", len(nodes), err)
	}
	sources, err := store.NodeRepo().FindNodeSources(context.Background(), nodes[0].ID)
	if err != nil || len(sources) != 1 || sources[0].UpstreamID != upstream.ID {
		t.Fatalf("unexpected source records: %v %+v", err, sources)
	}
}

func TestUpstreamRefresherFailureRecordsErrorAndKeepsExistingNodes(t *testing.T) {
	store, err := persistence.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "temporarily unavailable", http.StatusBadGateway)
	}))
	defer server.Close()
	now := time.Now().UTC()
	upstream := &domain.Upstream{ID: "upstream-2", Name: "test", URL: server.URL,
		Enabled: true, CreatedAt: now, UpdatedAt: now}
	if err := store.UpstreamRepo().Save(context.Background(), upstream); err != nil {
		t.Fatal(err)
	}
	existing := &domain.Node{ID: "existing", Name: "old", Protocol: "socks5", Address: "192.0.2.20", Port: 1080,
		Status: domain.NodeActive, CreatedAt: now, UpdatedAt: now, RawConfig: map[string]any{}}
	if err := store.NodeRepo().Save(context.Background(), existing); err != nil {
		t.Fatal(err)
	}

	refresher := NewUpstreamRefresher(store.UpstreamRepo(), store.NodeRepo(), store.MeasurementRepo(), server.Client())
	record, refreshErr := refresher.Refresh(context.Background(), upstream.ID)
	if refreshErr == nil || record.Success || !strings.Contains(record.Error, "HTTP 502") {
		t.Fatalf("expected recorded HTTP failure, record=%+v err=%v", record, refreshErr)
	}
	if _, err := store.NodeRepo().FindByID(context.Background(), existing.ID); err != nil {
		t.Fatalf("existing node was removed after failed refresh: %v", err)
	}
	records, err := store.UpstreamRepo().FindRefreshRecords(context.Background(), upstream.ID, 1)
	if err != nil || len(records) != 1 || records[0].Success {
		t.Fatalf("unexpected persisted refresh records: %v %+v", err, records)
	}
}

func TestUpstreamRefresherExpiresNodesRemovedFromSuccessfulRefresh(t *testing.T) {
	store, err := persistence.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	now := time.Now().UTC()
	upstream := &domain.Upstream{ID: "upstream-expire", Name: "test", URL: "http://unused", Enabled: true, CreatedAt: now, UpdatedAt: now}
	if err := store.UpstreamRepo().Save(context.Background(), upstream); err != nil {
		t.Fatal(err)
	}
	old := &domain.Node{ID: "old-node", Name: "old", Protocol: "socks5", Address: "192.0.2.30", Port: 1080, Status: domain.NodeActive, CreatedAt: now, UpdatedAt: now}
	if err := store.NodeRepo().Save(context.Background(), old); err != nil {
		t.Fatal(err)
	}
	if err := store.NodeRepo().SaveNodeSource(context.Background(), domain.NodeSource{NodeID: old.ID, UpstreamID: upstream.ID, CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := store.MeasurementRepo().Save(context.Background(), &domain.Measurement{ID: "expired-measurement", NodeID: old.ID, Kind: "throughput", CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("proxies:\n  - name: new\n    type: socks5\n    server: 192.0.2.31\n    port: 1080\n"))
	}))
	defer server.Close()
	upstream.URL = server.URL
	if err := store.UpstreamRepo().Save(context.Background(), upstream); err != nil {
		t.Fatal(err)
	}
	refresher := NewUpstreamRefresher(store.UpstreamRepo(), store.NodeRepo(), store.MeasurementRepo(), server.Client())
	if _, err := refresher.Refresh(context.Background(), upstream.ID); err != nil {
		t.Fatal(err)
	}
	got, err := store.NodeRepo().FindByID(context.Background(), old.ID)
	if err != nil || got.Status != domain.NodeExpired {
		t.Fatalf("expected old node expired, got %+v err=%v", got, err)
	}
	measurements, err := store.MeasurementRepo().FindByNodeID(context.Background(), old.ID, 10)
	if err != nil || len(measurements) != 0 {
		t.Fatalf("expected removed subscription node measurements to be deleted, got %d err=%v", len(measurements), err)
	}
}

func TestUpstreamRefresherDoesNotTakeOverManualNodeWithSameIdentity(t *testing.T) {
	store, err := persistence.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	body := "proxies:\n  - name: shared\n    type: socks5\n    server: 192.0.2.40\n    port: 1080\n"
	parsed, err := subscription_output.Parse([]byte(body), "manual")
	if err != nil || len(parsed.Nodes) != 1 {
		t.Fatalf("parse fixture: %v", err)
	}
	now := time.Now().UTC()
	manual := parsed.Nodes[0]
	manual.SourceURL = "manual"
	manual.Status = domain.NodeActive
	manual.CreatedAt = now
	manual.UpdatedAt = now
	if err := store.NodeRepo().Save(context.Background(), &manual); err != nil {
		t.Fatal(err)
	}

	currentBody := body
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(currentBody))
	}))
	defer server.Close()
	upstream := &domain.Upstream{ID: "upstream-manual-isolation", Name: "test", URL: server.URL,
		Enabled: true, CreatedAt: now, UpdatedAt: now}
	if err := store.UpstreamRepo().Save(context.Background(), upstream); err != nil {
		t.Fatal(err)
	}

	refresher := NewUpstreamRefresher(store.UpstreamRepo(), store.NodeRepo(), store.MeasurementRepo(), server.Client())
	if _, err := refresher.Refresh(context.Background(), upstream.ID); err != nil {
		t.Fatal(err)
	}
	currentBody = "proxies:\n  - name: renamed-by-subscription\n    type: socks5\n    server: 192.0.2.40\n    port: 1080\n"
	if _, err := refresher.Refresh(context.Background(), upstream.ID); err != nil {
		t.Fatal(err)
	}

	nodes, err := store.NodeRepo().FindAll(context.Background(), domain.NodeFilter{})
	if err != nil || len(nodes) != 2 {
		t.Fatalf("expected one manual and one subscription node, got %d, err=%v", len(nodes), err)
	}
	manualFound := false
	subscriptionFound := false
	for _, node := range nodes {
		sources, sourceErr := store.NodeRepo().FindNodeSources(context.Background(), node.ID)
		if sourceErr != nil {
			t.Fatal(sourceErr)
		}
		if node.SourceURL == "manual" && len(sources) == 0 {
			manualFound = true
			if node.Status != domain.NodeActive {
				t.Fatalf("manual node status changed: %s", node.Status)
			}
		}
		if len(sources) == 1 && sources[0].UpstreamID == upstream.ID {
			subscriptionFound = true
		}
	}
	if !manualFound || !subscriptionFound {
		t.Fatalf("manual/subscription ownership was not preserved: %+v", nodes)
	}
}
