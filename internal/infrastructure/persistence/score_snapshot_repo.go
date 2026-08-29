package persistence

import (
	"context"
	"database/sql"
	"errors"

	"github.com/teddymail/bagualu/internal/domain"
)

// ScoreSnapshotRepo is the SQLite implementation of domain.ScoreSnapshotRepository.
type ScoreSnapshotRepo struct{ db *sql.DB }

func NewScoreSnapshotRepo(db *sql.DB) *ScoreSnapshotRepo { return &ScoreSnapshotRepo{db: db} }

func (r *ScoreSnapshotRepo) Save(ctx context.Context, s *domain.ScoreSnapshot) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO score_snapshots(id,node_id,latency,speed,availability,overall,status,
			latency_samples,speed_samples,availability_samples,strategy_version,calculated_at)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(id) DO NOTHING`,
		s.ID, s.NodeID,
		s.Latency, s.Speed, s.Availability, s.Overall,
		string(s.Status),
		s.LatencySamples, s.SpeedSamples, s.AvailabilitySamples,
		s.StrategyVersion, encodeTime(s.CalculatedAt),
	)
	return err
}

func (r *ScoreSnapshotRepo) FindByNodeID(ctx context.Context, nodeID string, limit int) ([]domain.ScoreSnapshot, error) {
	q := `SELECT id,node_id,latency,speed,availability,overall,status,
		latency_samples,speed_samples,availability_samples,strategy_version,calculated_at
		FROM score_snapshots WHERE node_id=? ORDER BY calculated_at DESC`
	args := []interface{}{nodeID}
	if limit > 0 {
		q += " LIMIT ?"
		args = append(args, limit)
	}
	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSnapshots(rows)
}

func (r *ScoreSnapshotRepo) FindLatestByNodeID(ctx context.Context, nodeID string) (*domain.ScoreSnapshot, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id,node_id,latency,speed,availability,overall,status,
			latency_samples,speed_samples,availability_samples,strategy_version,calculated_at
		FROM score_snapshots WHERE node_id=? ORDER BY calculated_at DESC LIMIT 1`, nodeID)
	s, err := scanSnapshot(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	return s, err
}

type snapshotScanner interface {
	Scan(dest ...interface{}) error
}

func scanSnapshot(row snapshotScanner) (*domain.ScoreSnapshot, error) {
	var s domain.ScoreSnapshot
	var status, calculatedAt string
	if err := row.Scan(
		&s.ID, &s.NodeID,
		&s.Latency, &s.Speed, &s.Availability, &s.Overall,
		&status,
		&s.LatencySamples, &s.SpeedSamples, &s.AvailabilitySamples,
		&s.StrategyVersion, &calculatedAt,
	); err != nil {
		return nil, err
	}
	s.Status = domain.Recommendation(status)
	s.CalculatedAt = decodeTime(calculatedAt)
	return &s, nil
}

func scanSnapshots(rows *sql.Rows) ([]domain.ScoreSnapshot, error) {
	var snaps []domain.ScoreSnapshot
	for rows.Next() {
		s, err := scanSnapshot(rows)
		if err != nil {
			return nil, err
		}
		snaps = append(snaps, *s)
	}
	return snaps, rows.Err()
}
