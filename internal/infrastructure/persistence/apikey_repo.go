package persistence

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/teddymail/bagualu/internal/domain"
)

// APIKeyRepo is the SQLite implementation of domain.APIKeyRepository.
// The plaintext API key is NEVER stored; only the SHA-256 hex hash and display prefix are persisted.
type APIKeyRepo struct{ db *sql.DB }

func NewAPIKeyRepo(db *sql.DB) *APIKeyRepo { return &APIKeyRepo{db: db} }

func (r *APIKeyRepo) Save(ctx context.Context, k *domain.APIKey) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO api_keys(id,name,group_id,key_hash,prefix,expires_at,revoked_at,created_at,updated_at)
		VALUES(?,?,?,?,?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET
			name=excluded.name, group_id=excluded.group_id,
			key_hash=excluded.key_hash, prefix=excluded.prefix,
			expires_at=excluded.expires_at, revoked_at=excluded.revoked_at,
			updated_at=excluded.updated_at`,
		k.ID, k.Name, k.GroupID, k.KeyHash, k.Prefix,
		encodeOptTime(k.ExpiresAt), encodeOptTime(k.RevokedAt),
		encodeTime(k.CreatedAt), encodeTime(k.UpdatedAt),
	)
	return err
}

func (r *APIKeyRepo) FindByID(ctx context.Context, id string) (*domain.APIKey, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id,name,group_id,key_hash,prefix,expires_at,revoked_at,created_at,updated_at FROM api_keys WHERE id=?`, id)
	k, err := scanAPIKey(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	return k, err
}

func (r *APIKeyRepo) FindAll(ctx context.Context) ([]domain.APIKey, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id,name,group_id,key_hash,prefix,expires_at,revoked_at,created_at,updated_at FROM api_keys ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var keys []domain.APIKey
	for rows.Next() {
		k, err := scanAPIKey(rows)
		if err != nil {
			return nil, err
		}
		keys = append(keys, *k)
	}
	return keys, rows.Err()
}

func (r *APIKeyRepo) FindByKeyHash(ctx context.Context, hash string) (*domain.APIKey, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id,name,group_id,key_hash,prefix,expires_at,revoked_at,created_at,updated_at FROM api_keys WHERE key_hash=?`, hash)
	k, err := scanAPIKey(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	return k, err
}

func (r *APIKeyRepo) Revoke(ctx context.Context, id string, at time.Time) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE api_keys SET revoked_at=?, updated_at=? WHERE id=?`,
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

func (r *APIKeyRepo) Delete(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM api_keys WHERE id=?`, id)
	return err
}

type apiKeyScanner interface {
	Scan(dest ...interface{}) error
}

func scanAPIKey(row apiKeyScanner) (*domain.APIKey, error) {
	var k domain.APIKey
	var expiresAt, revokedAt sql.NullString
	var createdAt, updatedAt string
	if err := row.Scan(
		&k.ID, &k.Name, &k.GroupID, &k.KeyHash, &k.Prefix,
		&expiresAt, &revokedAt,
		&createdAt, &updatedAt,
	); err != nil {
		return nil, err
	}
	k.ExpiresAt = decodeOptTime(expiresAt)
	k.RevokedAt = decodeOptTime(revokedAt)
	k.CreatedAt = decodeTime(createdAt)
	k.UpdatedAt = decodeTime(updatedAt)
	return &k, nil
}
