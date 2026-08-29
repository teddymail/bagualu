package application

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/teddymail/bagualu/internal/domain"
	"github.com/teddymail/bagualu/internal/infrastructure/persistence"
)

func TestGeoServiceCachesLookupAndUpdatesNode(t *testing.T) {
	store, err := persistence.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	now := time.Now().UTC()
	if err := store.NodeRepo().Save(context.Background(), &domain.Node{ID: "geo-node", Name: "node", Protocol: "ss", Address: "192.0.2.1", Port: 443, Status: domain.NodeActive, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		_, _ = w.Write([]byte(`{"success":true,"country_code":"US","country":"United States","city":"New York","connection":{"asn":64500,"org":"Example ISP"}}`))
	}))
	defer server.Close()
	service := NewGeoService(store.NodeRepo(), NewHTTPGeoLookup(server.URL, server.Client()))
	if err := service.UpdateNode(context.Background(), "geo-node", "198.51.100.2"); err != nil {
		t.Fatal(err)
	}
	if err := service.UpdateNode(context.Background(), "geo-node", "198.51.100.2"); err != nil {
		t.Fatal(err)
	}
	node, err := store.NodeRepo().FindByID(context.Background(), "geo-node")
	if err != nil || node.Region != "US" || node.City != "New York" || node.ASN != "64500" || node.Organization != "Example ISP" || calls != 1 {
		t.Fatalf("unexpected geography: err=%v node=%+v calls=%d", err, node, calls)
	}
}
