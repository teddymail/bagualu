package persistence

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/teddymail/bagualu/internal/domain"
)

// SubscriptionLinkRepo is the SQLite implementation of domain.SubscriptionLinkRepository.
// The plaintext token is NEVER stored; only the SHA-256 hex hash and display prefix are persisted.
type SubscriptionLinkRepo struct{ db *sql.DB }

func NewSubscriptionLinkRepo(db *sql.DB) *SubscriptionLinkRepo {
	return &SubscriptionLinkRepo{db: db}
}

func (r *SubscriptionLinkRepo) Save(ctx context.Context, l *domain.SubscriptionLink) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO subscription_links(id,name,group_id,token_hash,token_prefix,default_format,
			min_score,max_nodes,healthy_only,enabled,expires_at,last_access_at,allowed_formats,created_at,updated_at)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET
			name=excluded.name, group_id=excluded.group_id,
			token_hash=excluded.token_hash, token_prefix=excluded.token_prefix,
			default_format=excluded.default_format, min_score=excluded.min_score,
			max_nodes=excluded.max_nodes, healthy_only=excluded.healthy_only,
			enabled=excluded.enabled, expires_at=excluded.expires_at,
			last_access_at=excluded.last_access_at, updated_at=excluded.updated_at`,
		l.ID, l.Name, l.GroupID, l.TokenHash, l.TokenPrefix,
		l.DefaultFormat, l.MinScore, l.Limit,
		boolToInt(l.HealthyOnly), boolToInt(l.Enabled),
		encodeOptTime(l.ExpiresAt), encodeOptTime(l.LastAccessAt), marshalStringList(l.AllowedFormats),
		encodeTime(l.CreatedAt), encodeTime(l.UpdatedAt),
	)
	return err
}

func (r *SubscriptionLinkRepo) FindByID(ctx context.Context, id string) (*domain.SubscriptionLink, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id,name,group_id,token_hash,token_prefix,default_format,min_score,max_nodes,
			healthy_only,enabled,expires_at,last_access_at,allowed_formats,created_at,updated_at
		FROM subscription_links WHERE id=?`, id)
	l, err := scanSubscriptionLink(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	return l, err
}

func (r *SubscriptionLinkRepo) FindAll(ctx context.Context) ([]domain.SubscriptionLink, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id,name,group_id,token_hash,token_prefix,default_format,min_score,max_nodes,
			healthy_only,enabled,expires_at,last_access_at,allowed_formats,created_at,updated_at
		FROM subscription_links ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var links []domain.SubscriptionLink
	for rows.Next() {
		l, err := scanSubscriptionLink(rows)
		if err != nil {
			return nil, err
		}
		links = append(links, *l)
	}
	return links, rows.Err()
}

func (r *SubscriptionLinkRepo) FindByTokenHash(ctx context.Context, hash string) (*domain.SubscriptionLink, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id,name,group_id,token_hash,token_prefix,default_format,min_score,max_nodes,
			healthy_only,enabled,expires_at,last_access_at,allowed_formats,created_at,updated_at
		FROM subscription_links WHERE token_hash=?`, hash)
	l, err := scanSubscriptionLink(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	return l, err
}

func (r *SubscriptionLinkRepo) UpdateLastAccess(ctx context.Context, id string, at time.Time) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE subscription_links SET last_access_at=?, updated_at=? WHERE id=?`,
		encodeTime(at), encodeTime(at), id,
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *SubscriptionLinkRepo) Delete(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM subscription_links WHERE id=?`, id)
	return err
}

type subscriptionLinkScanner interface {
	Scan(dest ...interface{}) error
}

func scanSubscriptionLink(row subscriptionLinkScanner) (*domain.SubscriptionLink, error) {
	var l domain.SubscriptionLink
	var healthyOnly, enabled int
	var expiresAt, lastAccessAt sql.NullString
	var allowedFormats string
	var createdAt, updatedAt string
	if err := row.Scan(
		&l.ID, &l.Name, &l.GroupID, &l.TokenHash, &l.TokenPrefix,
		&l.DefaultFormat, &l.MinScore, &l.Limit,
		&healthyOnly, &enabled,
		&expiresAt, &lastAccessAt, &allowedFormats,
		&createdAt, &updatedAt,
	); err != nil {
		return nil, err
	}
	l.HealthyOnly = healthyOnly != 0
	l.Enabled = enabled != 0
	l.ExpiresAt = decodeOptTime(expiresAt)
	l.LastAccessAt = decodeOptTime(lastAccessAt)
	_ = json.Unmarshal([]byte(allowedFormats), &l.AllowedFormats)
	l.CreatedAt = decodeTime(createdAt)
	l.UpdatedAt = decodeTime(updatedAt)
	return &l, nil
}

func marshalStringList(values []string) string {
	if len(values) == 0 {
		return ""
	}
	data, _ := json.Marshal(values)
	return string(data)
}
