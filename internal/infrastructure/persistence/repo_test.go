package persistence

import (
	"context"
	"testing"
	"time"

	"github.com/teddymail/bagualu/internal/domain"
)

// openTestStore opens an in-memory SQLite database with the full schema applied.
func openTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

var ctx = context.Background()

// ── NodeRepo ─────────────────────────────────────────────────────────────────

func TestNodeRepo_SaveAndFind(t *testing.T) {
	s := openTestStore(t)
	repo := s.NodeRepo()

	now := time.Now().UTC().Truncate(time.Second)
	n := &domain.Node{
		ID: "n1", Name: "test", Protocol: "vmess", Address: "1.2.3.4",
		Port: 443, Status: domain.NodeActive,
		RawConfig: map[string]any{"uuid": "abc"},
		CreatedAt: now, UpdatedAt: now,
	}
	if err := repo.Save(ctx, n); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := repo.FindByID(ctx, "n1")
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if got.Protocol != "vmess" || got.Port != 443 {
		t.Fatalf("unexpected node: %+v", got)
	}
	if uuid, _ := got.RawConfig["uuid"].(string); uuid != "abc" {
		t.Fatalf("raw_config not preserved: %v", got.RawConfig)
	}
}

func TestNodeRepo_NotFound(t *testing.T) {
	s := openTestStore(t)
	_, err := s.NodeRepo().FindByID(ctx, "missing")
	if err != domain.ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestNodeRepo_UpdateStatus(t *testing.T) {
	s := openTestStore(t)
	repo := s.NodeRepo()
	now := time.Now().UTC()
	_ = repo.Save(ctx, &domain.Node{ID: "n2", Name: "x", Protocol: "trojan", Address: "a", Port: 1,
		Status: domain.NodeActive, RawConfig: map[string]any{}, CreatedAt: now, UpdatedAt: now})
	if err := repo.UpdateStatus(ctx, "n2", domain.NodeDisabled); err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}
	got, _ := repo.FindByID(ctx, "n2")
	if got.Status != domain.NodeDisabled {
		t.Fatalf("status not updated: %v", got.Status)
	}
}

func TestNodeRepo_Delete(t *testing.T) {
	s := openTestStore(t)
	repo := s.NodeRepo()
	now := time.Now().UTC()
	_ = repo.Save(ctx, &domain.Node{ID: "n3", Name: "x", Protocol: "ss", Address: "a", Port: 1,
		Status: domain.NodeActive, RawConfig: map[string]any{}, CreatedAt: now, UpdatedAt: now})
	if err := repo.Delete(ctx, "n3"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := repo.FindByID(ctx, "n3"); err != domain.ErrNotFound {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestNodeRepo_NodeSources(t *testing.T) {
	s := openTestStore(t)
	repo := s.NodeRepo()
	uRepo := s.UpstreamRepo()
	now := time.Now().UTC()

	_ = repo.Save(ctx, &domain.Node{ID: "n4", Name: "x", Protocol: "vmess", Address: "a", Port: 1,
		Status: domain.NodeActive, RawConfig: map[string]any{}, CreatedAt: now, UpdatedAt: now})
	_ = uRepo.Save(ctx, &domain.Upstream{ID: "u1", Name: "sub", URL: "http://x",
		Format: domain.UpstreamFormatClash, RefreshInterval: time.Hour,
		Enabled: true, CreatedAt: now, UpdatedAt: now})

	src := domain.NodeSource{NodeID: "n4", UpstreamID: "u1", OriginalName: "orig", RawFragment: "raw", CreatedAt: now}
	if err := repo.SaveNodeSource(ctx, src); err != nil {
		t.Fatalf("SaveNodeSource: %v", err)
	}
	srcs, err := repo.FindNodeSources(ctx, "n4")
	if err != nil || len(srcs) != 1 {
		t.Fatalf("FindNodeSources: %v, len=%d", err, len(srcs))
	}
	if srcs[0].OriginalName != "orig" {
		t.Fatalf("unexpected source: %+v", srcs[0])
	}
}

func TestNodeRepo_FindAllFilter(t *testing.T) {
	s := openTestStore(t)
	repo := s.NodeRepo()
	now := time.Now().UTC()
	for i, proto := range []string{"vmess", "trojan"} {
		id := "fn" + string(rune('0'+i))
		_ = repo.Save(ctx, &domain.Node{ID: id, Name: id, Protocol: proto, Address: "a", Port: 1,
			Status: domain.NodeActive, RawConfig: map[string]any{}, CreatedAt: now, UpdatedAt: now})
	}
	nodes, err := repo.FindAll(ctx, domain.NodeFilter{Protocol: "vmess"})
	if err != nil || len(nodes) != 1 {
		t.Fatalf("FindAll(protocol=vmess): err=%v len=%d", err, len(nodes))
	}
}

// ── MeasurementRepo ───────────────────────────────────────────────────────────

func TestMeasurementRepo_SaveAndFind(t *testing.T) {
	s := openTestStore(t)
	now := time.Now().UTC().Truncate(time.Second)
	// insert parent node first
	_ = s.NodeRepo().Save(ctx, &domain.Node{ID: "m-node", Name: "x", Protocol: "ss", Address: "a", Port: 1,
		Status: domain.NodeActive, RawConfig: map[string]any{}, CreatedAt: now, UpdatedAt: now})

	repo := s.MeasurementRepo()
	m := &domain.Measurement{
		ID: "meas1", NodeID: "m-node", Kind: "connectivity", Success: true,
		LatencyMS: 55.5, CreatedAt: now,
		ProxyProtocol: "vless", TestURL: "https://example.test/file", SpeedSource: "https://example.test/file",
		CoreEvidence: domain.CoreEvidence{PID: "1234"},
	}
	if err := repo.Save(ctx, m); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := repo.FindByID(ctx, "meas1")
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if !got.Success || got.LatencyMS != 55.5 {
		t.Fatalf("unexpected measurement: %+v", got)
	}
	if got.CoreEvidence.PID != "1234" {
		t.Fatalf("evidence not round-tripped: %+v", got.CoreEvidence)
	}
	if got.ProxyProtocol != "vless" || got.TestURL != "https://example.test/file" || got.SpeedSource != "https://example.test/file" {
		t.Fatalf("measurement context not round-tripped: %+v", got)
	}
}

func TestMeasurementRepo_FindSince(t *testing.T) {
	s := openTestStore(t)
	now := time.Now().UTC().Truncate(time.Second)
	_ = s.NodeRepo().Save(ctx, &domain.Node{ID: "ms-node", Name: "x", Protocol: "ss", Address: "a", Port: 1,
		Status: domain.NodeActive, RawConfig: map[string]any{}, CreatedAt: now, UpdatedAt: now})

	repo := s.MeasurementRepo()
	old := &domain.Measurement{ID: "old", NodeID: "ms-node", Kind: "connectivity", CreatedAt: now.Add(-2 * time.Hour)}
	recent := &domain.Measurement{ID: "recent", NodeID: "ms-node", Kind: "connectivity", CreatedAt: now}
	_ = repo.Save(ctx, old)
	_ = repo.Save(ctx, recent)

	got, err := repo.FindSince(ctx, "ms-node", now.Add(-time.Minute))
	if err != nil || len(got) != 1 || got[0].ID != "recent" {
		t.Fatalf("FindSince: err=%v len=%d", err, len(got))
	}
}

func TestMeasurementRepo_DeleteByNodeID(t *testing.T) {
	s := openTestStore(t)
	now := time.Now().UTC().Truncate(time.Second)
	for _, id := range []string{"delete-node", "keep-node"} {
		if err := s.NodeRepo().Save(ctx, &domain.Node{ID: id, Name: id, Protocol: "ss", Address: "a", Port: 1,
			Status: domain.NodeActive, RawConfig: map[string]any{}, CreatedAt: now, UpdatedAt: now}); err != nil {
			t.Fatal(err)
		}
	}
	repo := s.MeasurementRepo()
	if err := repo.Save(ctx, &domain.Measurement{ID: "delete-measurement", NodeID: "delete-node", Kind: "ping", CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := repo.Save(ctx, &domain.Measurement{ID: "keep-measurement", NodeID: "keep-node", Kind: "ping", CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := repo.DeleteByNodeID(ctx, "delete-node"); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.FindByID(ctx, "delete-measurement"); err != domain.ErrNotFound {
		t.Fatalf("deleted node measurement still exists: %v", err)
	}
	if _, err := repo.FindByID(ctx, "keep-measurement"); err != nil {
		t.Fatalf("unrelated measurement was deleted: %v", err)
	}
}

func TestReportRepo_CleanupWithRetention(t *testing.T) {
	s := openTestStore(t)
	now := time.Now().UTC().Truncate(time.Second)
	upstream := &domain.Upstream{ID: "retention-upstream", Name: "subscription", URL: "http://subscription",
		Enabled: true, CreatedAt: now, UpdatedAt: now}
	if err := s.UpstreamRepo().Save(ctx, upstream); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"retention-subscription", "retention-manual"} {
		if err := s.NodeRepo().Save(ctx, &domain.Node{ID: id, Name: id, Protocol: "ss", Address: "a", Port: 1,
			Status: domain.NodeActive, RawConfig: map[string]any{}, CreatedAt: now, UpdatedAt: now}); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.NodeRepo().SaveNodeSource(ctx, domain.NodeSource{NodeID: "retention-subscription", UpstreamID: upstream.ID, CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	measurements := []*domain.Measurement{
		{ID: "subscription-old", NodeID: "retention-subscription", Kind: "ping", CreatedAt: now.Add(-8 * 24 * time.Hour)},
		{ID: "subscription-recent", NodeID: "retention-subscription", Kind: "ping", CreatedAt: now.Add(-6 * 24 * time.Hour)},
		{ID: "manual-old", NodeID: "retention-manual", Kind: "ping", CreatedAt: now.Add(-31 * 24 * time.Hour)},
		{ID: "manual-recent", NodeID: "retention-manual", Kind: "ping", CreatedAt: now.Add(-8 * 24 * time.Hour)},
	}
	for _, measurement := range measurements {
		if err := s.MeasurementRepo().Save(ctx, measurement); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.ReportRepo().CleanupWithRetention(ctx, now.Add(-30*24*time.Hour), now.Add(-7*24*time.Hour), now.Add(time.Hour), now.Add(time.Hour), now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		id     string
		exists bool
	}{
		{id: "subscription-old", exists: false},
		{id: "subscription-recent", exists: true},
		{id: "manual-old", exists: false},
		{id: "manual-recent", exists: true},
	} {
		_, err := s.MeasurementRepo().FindByID(ctx, test.id)
		if test.exists && err != nil {
			t.Errorf("measurement %s should remain: %v", test.id, err)
		}
		if !test.exists && err != domain.ErrNotFound {
			t.Errorf("measurement %s should be deleted, got %v", test.id, err)
		}
	}
}

// ── UpstreamRepo ──────────────────────────────────────────────────────────────

func TestUpstreamRepo_CRUD(t *testing.T) {
	s := openTestStore(t)
	repo := s.UpstreamRepo()
	now := time.Now().UTC().Truncate(time.Second)

	u := &domain.Upstream{
		ID: "up1", Name: "MyFeed", URL: "https://example.com/sub",
		Format: domain.UpstreamFormatClash, RefreshInterval: 2 * time.Hour,
		Enabled: true, Notes: "test", CreatedAt: now, UpdatedAt: now,
	}
	if err := repo.Save(ctx, u); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := repo.FindByID(ctx, "up1")
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if got.RefreshInterval != 2*time.Hour || got.Format != domain.UpstreamFormatClash {
		t.Fatalf("unexpected upstream: %+v", got)
	}

	u.Name = "Updated"
	u.UpdatedAt = now.Add(time.Minute)
	if err := repo.Save(ctx, u); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	got2, _ := repo.FindByID(ctx, "up1")
	if got2.Name != "Updated" {
		t.Fatalf("name not updated: %v", got2.Name)
	}

	all, err := repo.FindAll(ctx)
	if err != nil || len(all) != 1 {
		t.Fatalf("FindAll: %v, len=%d", err, len(all))
	}

	if err := repo.Delete(ctx, "up1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := repo.FindByID(ctx, "up1"); err != domain.ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestUpstreamRepo_RefreshRecords(t *testing.T) {
	s := openTestStore(t)
	repo := s.UpstreamRepo()
	now := time.Now().UTC().Truncate(time.Second)

	_ = repo.Save(ctx, &domain.Upstream{ID: "up2", Name: "x", URL: "http://x",
		Format: domain.UpstreamFormatBase64, RefreshInterval: time.Hour,
		Enabled: true, CreatedAt: now, UpdatedAt: now})

	rec := &domain.RefreshRecord{ID: "r1", UpstreamID: "up2", Success: true, NodeCount: 5, CreatedAt: now}
	if err := repo.SaveRefreshRecord(ctx, rec); err != nil {
		t.Fatalf("SaveRefreshRecord: %v", err)
	}
	recs, err := repo.FindRefreshRecords(ctx, "up2", 10)
	if err != nil || len(recs) != 1 || !recs[0].Success {
		t.Fatalf("FindRefreshRecords: %v len=%d", err, len(recs))
	}
}

// ── GroupRepo ─────────────────────────────────────────────────────────────────

func TestGroupRepo_CRUD(t *testing.T) {
	s := openTestStore(t)
	repo := s.GroupRepo()
	now := time.Now().UTC().Truncate(time.Second)

	g := &domain.Group{ID: "g1", Name: "Default", Description: "desc",
		MinScore: 65, OnePerEndpointIP: true, CreatedAt: now, UpdatedAt: now}
	if err := repo.Save(ctx, g); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := repo.FindByID(ctx, "g1")
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if got.MinScore != 65 || !got.OnePerEndpointIP {
		t.Fatalf("unexpected group: %+v", got)
	}
	if err := repo.Delete(ctx, "g1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
}

func TestGroupRepo_SetNodes(t *testing.T) {
	s := openTestStore(t)
	nRepo := s.NodeRepo()
	gRepo := s.GroupRepo()
	now := time.Now().UTC()

	for _, id := range []string{"gn1", "gn2", "gn3"} {
		_ = nRepo.Save(ctx, &domain.Node{ID: id, Name: id, Protocol: "ss", Address: "a", Port: 1,
			Status: domain.NodeActive, RawConfig: map[string]any{}, CreatedAt: now, UpdatedAt: now})
	}
	_ = gRepo.Save(ctx, &domain.Group{ID: "grp1", Name: "g", MinScore: 60, OnePerEndpointIP: true,
		CreatedAt: now, UpdatedAt: now})

	if err := gRepo.SetNodes(ctx, "grp1", []string{"gn1", "gn2"}); err != nil {
		t.Fatalf("SetNodes: %v", err)
	}
	ids, err := gRepo.FindNodeIDs(ctx, "grp1")
	if err != nil || len(ids) != 2 {
		t.Fatalf("FindNodeIDs: err=%v len=%d", err, len(ids))
	}

	// Replace with single node
	if err := gRepo.SetNodes(ctx, "grp1", []string{"gn3"}); err != nil {
		t.Fatalf("SetNodes replace: %v", err)
	}
	ids2, _ := gRepo.FindNodeIDs(ctx, "grp1")
	if len(ids2) != 1 || ids2[0] != "gn3" {
		t.Fatalf("SetNodes did not replace: %v", ids2)
	}
}

// ── JobRepo ───────────────────────────────────────────────────────────────────

func TestJobRepo_CRUD(t *testing.T) {
	s := openTestStore(t)
	repo := s.JobRepo()
	now := time.Now().UTC().Truncate(time.Second)

	j := &domain.Job{ID: "j1", Kind: "refresh_upstream", Status: domain.JobPending,
		EntityID: "up1", CreatedAt: now, UpdatedAt: now}
	if err := repo.Save(ctx, j); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := repo.FindByID(ctx, "j1")
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if got.Kind != "refresh_upstream" || got.Status != domain.JobPending {
		t.Fatalf("unexpected job: %+v", got)
	}
	if err := repo.UpdateStatusDetail(ctx, "j1", domain.JobFailed, 100,
		domain.ErrCodeCoreAPIUnavailable, "Mihomo returned HTTP 503 while loading the node", "core_api"); err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}
	got2, _ := repo.FindByID(ctx, "j1")
	if got2.Status != domain.JobFailed || got2.FinishedAt == nil ||
		got2.ErrorCode != domain.ErrCodeCoreAPIUnavailable ||
		got2.ErrorDetail != "Mihomo returned HTTP 503 while loading the node" ||
		got2.FailureStage != "core_api" {
		t.Fatalf("status not updated: %+v", got2)
	}
}

func TestJobRepo_FindAllFilter(t *testing.T) {
	s := openTestStore(t)
	repo := s.JobRepo()
	now := time.Now().UTC()

	for i, kind := range []string{"refresh_upstream", "test_node"} {
		id := "jf" + string(rune('0'+i))
		_ = repo.Save(ctx, &domain.Job{ID: id, Kind: kind, Status: domain.JobPending,
			CreatedAt: now, UpdatedAt: now})
	}
	jobs, err := repo.FindAll(ctx, domain.JobFilter{Kind: "test_node"})
	if err != nil || len(jobs) != 1 {
		t.Fatalf("FindAll(kind=test_node): err=%v len=%d", err, len(jobs))
	}
}

func TestJobRepo_DeleteInactive(t *testing.T) {
	s := openTestStore(t)
	repo := s.JobRepo()
	now := time.Now().UTC()
	for _, job := range []*domain.Job{
		{ID: "active-job", Kind: "test_ping", Status: domain.JobRunning, CreatedAt: now, UpdatedAt: now},
		{ID: "done-job", Kind: "test_ping", Status: domain.JobSucceeded, CreatedAt: now, UpdatedAt: now},
	} {
		if err := repo.Save(ctx, job); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.DB.Exec(`INSERT INTO jobs(id,kind,status,created_at,updated_at) VALUES(?,?,?,?,?)`, "unknown-job", "test_ping", "unknown", encodeTime(now), encodeTime(now)); err != nil {
		t.Fatal(err)
	}
	if err := repo.DeleteInactive(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.FindByID(ctx, "active-job"); err != nil {
		t.Fatalf("active job was deleted: %v", err)
	}
	for _, id := range []string{"done-job", "unknown-job"} {
		if _, err := repo.FindByID(ctx, id); err != domain.ErrNotFound {
			t.Fatalf("inactive job %s still exists: %v", id, err)
		}
	}
}

func TestJobRepoCancelOrphanedActive(t *testing.T) {
	s := openTestStore(t)
	repo := s.JobRepo()
	now := time.Now().UTC().Truncate(time.Second)
	for _, job := range []*domain.Job{
		{ID: "orphan-running", Kind: "test_ping", Status: domain.JobRunning, CreatedAt: now, UpdatedAt: now},
		{ID: "finished", Kind: "test_ping", Status: domain.JobSucceeded, CreatedAt: now, UpdatedAt: now},
	} {
		if err := repo.Save(ctx, job); err != nil {
			t.Fatal(err)
		}
	}
	if err := repo.CancelOrphanedActive(ctx); err != nil {
		t.Fatal(err)
	}
	orphan, err := repo.FindByID(ctx, "orphan-running")
	if err != nil || orphan.Status != domain.JobCancelled || orphan.Error != "service_restarted" {
		t.Fatalf("orphan was not cancelled: %+v err=%v", orphan, err)
	}
	finished, err := repo.FindByID(ctx, "finished")
	if err != nil || finished.Status != domain.JobSucceeded {
		t.Fatalf("terminal job was changed: %+v err=%v", finished, err)
	}
}

// ── APIKeyRepo ────────────────────────────────────────────────────────────────

func TestAPIKeyRepo_CRUD(t *testing.T) {
	s := openTestStore(t)
	repo := s.APIKeyRepo()
	now := time.Now().UTC().Truncate(time.Second)

	k := &domain.APIKey{
		ID: "k1", Name: "client-a", GroupID: "g1",
		KeyHash: "sha256hex1234", Prefix: "bg_12345678",
		CreatedAt: now, UpdatedAt: now,
	}
	if err := repo.Save(ctx, k); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := repo.FindByID(ctx, "k1")
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if got.KeyHash != "sha256hex1234" || got.Prefix != "bg_12345678" {
		t.Fatalf("unexpected key: %+v", got)
	}
	byHash, err := repo.FindByKeyHash(ctx, "sha256hex1234")
	if err != nil || byHash.ID != "k1" {
		t.Fatalf("FindByKeyHash: %v %+v", err, byHash)
	}
	if err := repo.Revoke(ctx, "k1", now.Add(time.Hour)); err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	got2, _ := repo.FindByID(ctx, "k1")
	if got2.RevokedAt == nil {
		t.Fatalf("key not revoked: %+v", got2)
	}
}

func TestAPIKeyRepo_NotFound(t *testing.T) {
	s := openTestStore(t)
	_, err := s.APIKeyRepo().FindByKeyHash(ctx, "nonexistent")
	if err != domain.ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

// ── SubscriptionLinkRepo ──────────────────────────────────────────────────────

func TestSubscriptionLinkRepo_CRUD(t *testing.T) {
	s := openTestStore(t)
	repo := s.SubscriptionLinkRepo()
	now := time.Now().UTC().Truncate(time.Second)

	l := &domain.SubscriptionLink{
		ID: "sl1", Name: "my-link", GroupID: "g1",
		TokenHash: "tokenhash123", TokenPrefix: "tk_abcd",
		DefaultFormat: "clash", MinScore: 60, Limit: 50,
		HealthyOnly: true, Enabled: true,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := repo.Save(ctx, l); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := repo.FindByID(ctx, "sl1")
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if got.DefaultFormat != "clash" || got.Limit != 50 {
		t.Fatalf("unexpected link: %+v", got)
	}
	byHash, err := repo.FindByTokenHash(ctx, "tokenhash123")
	if err != nil || byHash.ID != "sl1" {
		t.Fatalf("FindByTokenHash: %v %+v", err, byHash)
	}
	accessAt := now.Add(time.Hour)
	if err := repo.UpdateLastAccess(ctx, "sl1", accessAt); err != nil {
		t.Fatalf("UpdateLastAccess: %v", err)
	}
	got2, _ := repo.FindByID(ctx, "sl1")
	if got2.LastAccessAt == nil || !got2.LastAccessAt.Equal(accessAt) {
		t.Fatalf("last access not updated: %+v", got2)
	}
	if err := repo.Delete(ctx, "sl1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := repo.FindByID(ctx, "sl1"); err != domain.ErrNotFound {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}
}

// ── SettingsRepo ──────────────────────────────────────────────────────────────

func TestSettingsRepo_SetGetDelete(t *testing.T) {
	s := openTestStore(t)
	repo := s.SettingsRepo()

	if err := repo.Set(ctx, "admin_password_hash", "$2a$10$hash"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	val, err := repo.Get(ctx, "admin_password_hash")
	if err != nil || val != "$2a$10$hash" {
		t.Fatalf("Get: %v %q", err, val)
	}

	// Overwrite
	if err := repo.Set(ctx, "admin_password_hash", "new_hash"); err != nil {
		t.Fatalf("Set overwrite: %v", err)
	}
	val2, _ := repo.Get(ctx, "admin_password_hash")
	if val2 != "new_hash" {
		t.Fatalf("overwrite failed: %q", val2)
	}

	all, err := repo.GetAll(ctx)
	if err != nil || all["admin_password_hash"] != "new_hash" {
		t.Fatalf("GetAll: %v %v", err, all)
	}

	if _, err := repo.Get(ctx, "missing"); err != domain.ErrNotFound {
		t.Fatalf("expected ErrNotFound for missing key, got %v", err)
	}

	if err := repo.Delete(ctx, "admin_password_hash"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := repo.Get(ctx, "admin_password_hash"); err != domain.ErrNotFound {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}
}

// ── ScoreSnapshotRepo ─────────────────────────────────────────────────────────

func TestScoreSnapshotRepo_SaveAndFind(t *testing.T) {
	s := openTestStore(t)
	nRepo := s.NodeRepo()
	ssRepo := s.ScoreSnapshotRepo()
	now := time.Now().UTC().Truncate(time.Second)

	_ = nRepo.Save(ctx, &domain.Node{ID: "ss-node", Name: "x", Protocol: "vmess", Address: "a",
		Port: 1, Status: domain.NodeActive, RawConfig: map[string]any{},
		CreatedAt: now, UpdatedAt: now})

	snap1 := &domain.ScoreSnapshot{
		ID: "ss1", NodeID: "ss-node",
		Latency: 80, Speed: 70, Availability: 90, Overall: 78,
		Status:         domain.RecommendationRecommended,
		LatencySamples: 10, SpeedSamples: 3, AvailabilitySamples: 20,
		StrategyVersion: 1, CalculatedAt: now.Add(-time.Hour),
	}
	snap2 := &domain.ScoreSnapshot{
		ID: "ss2", NodeID: "ss-node",
		Latency: 85, Speed: 75, Availability: 95, Overall: 82,
		Status:         domain.RecommendationRecommended,
		LatencySamples: 12, SpeedSamples: 3, AvailabilitySamples: 22,
		StrategyVersion: 1, CalculatedAt: now,
	}
	for _, snap := range []*domain.ScoreSnapshot{snap1, snap2} {
		if err := ssRepo.Save(ctx, snap); err != nil {
			t.Fatalf("Save: %v", err)
		}
	}

	snaps, err := ssRepo.FindByNodeID(ctx, "ss-node", 10)
	if err != nil || len(snaps) != 2 {
		t.Fatalf("FindByNodeID: err=%v len=%d", err, len(snaps))
	}
	// Newest first
	if snaps[0].ID != "ss2" {
		t.Fatalf("unexpected order: %v", snaps[0].ID)
	}

	latest, err := ssRepo.FindLatestByNodeID(ctx, "ss-node")
	if err != nil || latest.ID != "ss2" {
		t.Fatalf("FindLatestByNodeID: err=%v id=%v", err, latest)
	}
}

func TestScoreSnapshotRepo_NotFound(t *testing.T) {
	s := openTestStore(t)
	_, err := s.ScoreSnapshotRepo().FindLatestByNodeID(ctx, "no-such-node")
	if err != domain.ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}
