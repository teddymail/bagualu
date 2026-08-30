package persistence

import (
	"database/sql"
	"embed"
	"fmt"
	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schemaFS embed.FS

type Store struct{ DB *sql.DB }

func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	if _, err = db.Exec(`PRAGMA busy_timeout=5000`); err != nil {
		db.Close()
		return nil, fmt.Errorf("configure database busy timeout: %w", err)
	}
	if path != ":memory:" {
		if _, err = db.Exec(`PRAGMA journal_mode=WAL`); err != nil {
			db.Close()
			return nil, fmt.Errorf("configure database journal mode: %w", err)
		}
	}
	schema, err := schemaFS.ReadFile("schema.sql")
	if err != nil {
		db.Close()
		return nil, err
	}
	if _, err = db.Exec(string(schema)); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate database: %w", err)
	}
	// Keep databases created by earlier builds readable after adding optional
	// subscription policy fields.
	_, _ = db.Exec(`ALTER TABLE subscription_links ADD COLUMN allowed_formats TEXT NOT NULL DEFAULT ''`)
	_, _ = db.Exec(`ALTER TABLE nodes ADD COLUMN exit_ip TEXT NOT NULL DEFAULT ''`)
	_, _ = db.Exec(`ALTER TABLE nodes ADD COLUMN country TEXT NOT NULL DEFAULT ''`)
	_, _ = db.Exec(`ALTER TABLE nodes ADD COLUMN city TEXT NOT NULL DEFAULT ''`)
	_, _ = db.Exec(`ALTER TABLE nodes ADD COLUMN asn TEXT NOT NULL DEFAULT ''`)
	_, _ = db.Exec(`ALTER TABLE nodes ADD COLUMN organization TEXT NOT NULL DEFAULT ''`)
	_, _ = db.Exec(`ALTER TABLE measurements ADD COLUMN context_json TEXT NOT NULL DEFAULT '{}'`)
	_, _ = db.Exec(`ALTER TABLE jobs ADD COLUMN error_code TEXT NOT NULL DEFAULT ''`)
	_, _ = db.Exec(`ALTER TABLE jobs ADD COLUMN error_detail TEXT NOT NULL DEFAULT ''`)
	_, _ = db.Exec(`ALTER TABLE jobs ADD COLUMN failure_stage TEXT NOT NULL DEFAULT ''`)
	_, _ = db.Exec(`ALTER TABLE nodes ADD COLUMN geo_source TEXT NOT NULL DEFAULT ''`)
	_, _ = db.Exec(`ALTER TABLE nodes ADD COLUMN geo_updated_at TEXT`)
	_, _ = db.Exec(`ALTER TABLE nodes ADD COLUMN region_changed_at TEXT`)
	return &Store{DB: db}, nil
}

func (s *Store) Close() error { return s.DB.Close() }

// Repository factory methods — callers typically obtain these via the Store.

func (s *Store) NodeRepo() *NodeRepo               { return NewNodeRepo(s.DB) }
func (s *Store) MeasurementRepo() *MeasurementRepo { return NewMeasurementRepo(s.DB) }
func (s *Store) UpstreamRepo() *UpstreamRepo       { return NewUpstreamRepo(s.DB) }
func (s *Store) GroupRepo() *GroupRepo             { return NewGroupRepo(s.DB) }
func (s *Store) JobRepo() *JobRepo                 { return NewJobRepo(s.DB) }
func (s *Store) APIKeyRepo() *APIKeyRepo           { return NewAPIKeyRepo(s.DB) }
func (s *Store) SubscriptionLinkRepo() *SubscriptionLinkRepo {
	return NewSubscriptionLinkRepo(s.DB)
}
func (s *Store) SettingsRepo() *SettingsRepo           { return NewSettingsRepo(s.DB) }
func (s *Store) ScoreSnapshotRepo() *ScoreSnapshotRepo { return NewScoreSnapshotRepo(s.DB) }
func (s *Store) ReportRepo() *ReportRepo               { return NewReportRepo(s.DB) }
