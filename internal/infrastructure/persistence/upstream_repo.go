package persistence

import (
	"context"
	"database/sql"
	"errors"

	"github.com/teddymail/bagualu/internal/domain"
)

// UpstreamRepo is the SQLite implementation of domain.UpstreamRepository.
type UpstreamRepo struct{ db *sql.DB }

func NewUpstreamRepo(db *sql.DB) *UpstreamRepo { return &UpstreamRepo{db: db} }

func (r *UpstreamRepo) Save(ctx context.Context, u *domain.Upstream) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO upstreams(id,name,url,format,refresh_interval_sec,enabled,notes,created_at,updated_at)
		VALUES(?,?,?,?,?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET
			name=excluded.name, url=excluded.url, format=excluded.format,
			refresh_interval_sec=excluded.refresh_interval_sec, enabled=excluded.enabled,
			notes=excluded.notes, updated_at=excluded.updated_at`,
		u.ID, u.Name, u.URL, string(u.Format),
		int64(u.RefreshInterval.Seconds()),
		boolToInt(u.Enabled), u.Notes,
		encodeTime(u.CreatedAt), encodeTime(u.UpdatedAt),
	)
	return err
}

func (r *UpstreamRepo) FindByID(ctx context.Context, id string) (*domain.Upstream, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id,name,url,format,refresh_interval_sec,enabled,notes,created_at,updated_at
		 FROM upstreams WHERE id=?`, id)
	u, err := scanUpstream(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	return u, err
}

func (r *UpstreamRepo) FindAll(ctx context.Context) ([]domain.Upstream, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id,name,url,format,refresh_interval_sec,enabled,notes,created_at,updated_at
		 FROM upstreams ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var us []domain.Upstream
	for rows.Next() {
		u, err := scanUpstream(rows)
		if err != nil {
			return nil, err
		}
		us = append(us, *u)
	}
	return us, rows.Err()
}

func (r *UpstreamRepo) Delete(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM upstreams WHERE id=?`, id)
	return err
}

func (r *UpstreamRepo) SaveRefreshRecord(ctx context.Context, rec *domain.RefreshRecord) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO refresh_records(id,upstream_id,success,error,node_count,created_at)
		VALUES(?,?,?,?,?,?)
		ON CONFLICT(id) DO NOTHING`,
		rec.ID, rec.UpstreamID, boolToInt(rec.Success),
		rec.Error, rec.NodeCount, encodeTime(rec.CreatedAt),
	)
	return err
}

func (r *UpstreamRepo) FindRefreshRecords(ctx context.Context, upstreamID string, limit int) ([]domain.RefreshRecord, error) {
	q := `SELECT id,upstream_id,success,error,node_count,created_at FROM refresh_records WHERE upstream_id=? ORDER BY created_at DESC`
	args := []interface{}{upstreamID}
	if limit > 0 {
		q += " LIMIT ?"
		args = append(args, limit)
	}
	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var recs []domain.RefreshRecord
	for rows.Next() {
		var rec domain.RefreshRecord
		var success int
		var createdAt string
		if err := rows.Scan(&rec.ID, &rec.UpstreamID, &success, &rec.Error, &rec.NodeCount, &createdAt); err != nil {
			return nil, err
		}
		rec.Success = success != 0
		rec.CreatedAt = decodeTime(createdAt)
		recs = append(recs, rec)
	}
	return recs, rows.Err()
}

type upstreamScanner interface {
	Scan(dest ...interface{}) error
}

func scanUpstream(row upstreamScanner) (*domain.Upstream, error) {
	var u domain.Upstream
	var format string
	var refreshIntervalSec int64
	var enabled int
	var createdAt, updatedAt string
	err := row.Scan(
		&u.ID, &u.Name, &u.URL, &format,
		&refreshIntervalSec, &enabled, &u.Notes,
		&createdAt, &updatedAt,
	)
	if err != nil {
		return nil, err
	}
	u.Format = domain.UpstreamFormat(format)
	u.RefreshInterval = durationFromSec(refreshIntervalSec)
	u.Enabled = enabled != 0
	u.CreatedAt = decodeTime(createdAt)
	u.UpdatedAt = decodeTime(updatedAt)
	return &u, nil
}
