package persistence

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/teddymail/bagualu/internal/domain"
)

// GroupRepo is the SQLite implementation of domain.GroupRepository.
type GroupRepo struct{ db *sql.DB }

func NewGroupRepo(db *sql.DB) *GroupRepo { return &GroupRepo{db: db} }

func (r *GroupRepo) Save(ctx context.Context, g *domain.Group) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO groups(id,name,description,min_score,one_per_endpoint_ip,created_at,updated_at)
		VALUES(?,?,?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET
			name=excluded.name, description=excluded.description,
			min_score=excluded.min_score, one_per_endpoint_ip=excluded.one_per_endpoint_ip,
			updated_at=excluded.updated_at`,
		g.ID, g.Name, g.Description, g.MinScore,
		boolToInt(g.OnePerEndpointIP),
		encodeTime(g.CreatedAt), encodeTime(g.UpdatedAt),
	)
	return err
}

func (r *GroupRepo) FindByID(ctx context.Context, id string) (*domain.Group, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id,name,description,min_score,one_per_endpoint_ip,created_at,updated_at FROM groups WHERE id=?`, id)
	g, err := scanGroup(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	return g, err
}

func (r *GroupRepo) FindAll(ctx context.Context) ([]domain.Group, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id,name,description,min_score,one_per_endpoint_ip,created_at,updated_at FROM groups ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var gs []domain.Group
	for rows.Next() {
		g, err := scanGroup(rows)
		if err != nil {
			return nil, err
		}
		gs = append(gs, *g)
	}
	return gs, rows.Err()
}

func (r *GroupRepo) Delete(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM groups WHERE id=?`, id)
	return err
}

// SetNodes atomically replaces the node membership of a group.
func (r *GroupRepo) SetNodes(ctx context.Context, groupID string, nodeIDs []string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck

	if _, err := tx.ExecContext(ctx, `DELETE FROM node_groups WHERE group_id=?`, groupID); err != nil {
		return err
	}
	for _, nid := range nodeIDs {
		if _, err := tx.ExecContext(ctx,
			`INSERT OR IGNORE INTO node_groups(node_id,group_id) VALUES(?,?)`, nid, groupID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *GroupRepo) FindNodeIDs(ctx context.Context, groupID string) ([]string, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT node_id FROM node_groups WHERE group_id=? ORDER BY node_id`, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

type groupScanner interface {
	Scan(dest ...interface{}) error
}

func scanGroup(row groupScanner) (*domain.Group, error) {
	var g domain.Group
	var onePerIP int
	var createdAt, updatedAt string
	if err := row.Scan(
		&g.ID, &g.Name, &g.Description, &g.MinScore, &onePerIP,
		&createdAt, &updatedAt,
	); err != nil {
		return nil, err
	}
	g.OnePerEndpointIP = onePerIP != 0
	g.CreatedAt = decodeTime(createdAt)
	g.UpdatedAt = decodeTime(updatedAt)
	return &g, nil
}

// durationFromSec converts seconds to time.Duration.
func durationFromSec(sec int64) time.Duration {
	return time.Duration(sec) * time.Second
}
