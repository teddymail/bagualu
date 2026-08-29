package application

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/teddymail/bagualu/internal/domain"
)

type GeoResult struct {
	CountryCode  string
	Country      string
	City         string
	ASN          string
	Organization string
}

type GeoLookup interface {
	Lookup(context.Context, string) (GeoResult, error)
}

type HTTPGeoLookup struct {
	client  *http.Client
	baseURL string
}

func NewHTTPGeoLookup(baseURL string, client *http.Client) *HTTPGeoLookup {
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}
	if baseURL == "" {
		baseURL = "https://ipwho.is/"
	}
	return &HTTPGeoLookup{client: client, baseURL: strings.TrimRight(baseURL, "/") + "/"}
}

func (l *HTTPGeoLookup) Lookup(ctx context.Context, ip string) (GeoResult, error) {
	if strings.TrimSpace(ip) == "" {
		return GeoResult{}, fmt.Errorf("empty exit IP")
	}
	endpoint := l.baseURL + url.PathEscape(ip)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return GeoResult{}, err
	}
	response, err := l.client.Do(request)
	if err != nil {
		return GeoResult{}, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return GeoResult{}, fmt.Errorf("geolocation returned HTTP %d", response.StatusCode)
	}
	var payload struct {
		Success     bool   `json:"success"`
		CountryCode string `json:"country_code"`
		Country     string `json:"country"`
		City        string `json:"city"`
		Connection  struct {
			ASN any    `json:"asn"`
			Org string `json:"org"`
		} `json:"connection"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 32<<10)).Decode(&payload); err != nil {
		return GeoResult{}, err
	}
	if !payload.Success && payload.CountryCode == "" {
		return GeoResult{}, fmt.Errorf("geolocation lookup failed")
	}
	return GeoResult{CountryCode: payload.CountryCode, Country: payload.Country, City: payload.City, ASN: fmt.Sprint(payload.Connection.ASN), Organization: payload.Connection.Org}, nil
}

type GeoService struct {
	nodes  domain.NodeRepository
	lookup GeoLookup
	mu     sync.Mutex
	cache  map[string]GeoResult
}

func NewGeoService(nodes domain.NodeRepository, lookup GeoLookup) *GeoService {
	return &GeoService{nodes: nodes, lookup: lookup, cache: make(map[string]GeoResult)}
}

func (s *GeoService) UpdateNode(ctx context.Context, nodeID, exitIP string) error {
	if s.nodes == nil || s.lookup == nil {
		return fmt.Errorf("geolocation is not configured")
	}
	parsedIP := net.ParseIP(strings.TrimSpace(exitIP))
	if parsedIP == nil {
		return fmt.Errorf("invalid exit IP %q", exitIP)
	}
	exitIP = parsedIP.String()
	node, err := s.nodes.FindByID(ctx, nodeID)
	if err != nil {
		return err
	}
	s.mu.Lock()
	result, ok := s.cache[exitIP]
	s.mu.Unlock()
	if !ok {
		result, err = s.lookup.Lookup(ctx, exitIP)
		if err != nil {
			return err
		}
		s.mu.Lock()
		s.cache[exitIP] = result
		s.mu.Unlock()
	}
	previousRegion := node.Region
	now := time.Now().UTC()
	node.ExitIP = exitIP
	node.Country = result.Country
	node.City = result.City
	node.ASN = result.ASN
	node.Organization = result.Organization
	node.Region = result.CountryCode
	node.GeoSource = "ipwho.is"
	node.GeoUpdatedAt = &now
	if previousRegion != node.Region {
		node.RegionChangedAt = &now
	}
	node.UpdatedAt = time.Now().UTC()
	return s.nodes.Save(ctx, node)
}
