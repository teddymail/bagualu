package persistence

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/teddymail/bagualu/internal/domain"
)

// JobRepo is the SQLite implementation of domain.JobRepository.
type JobRepo struct{ db *sql.DB }

func NewJobRepo(db *sql.DB) *JobRepo { return &JobRepo{db: db} }

func (r *JobRepo) Save(ctx context.Context, j *domain.Job) error {
	if !validJobStatus(j.Status) {
		return fmt.Errorf("invalid job status %q", j.Status)
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO jobs(id,kind,status,progress,entity_id,error,created_at,updated_at,finished_at)
		VALUES(?,?,?,?,?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET
			kind=excluded.kind, status=excluded.status, progress=excluded.progress,
			entity_id=excluded.entity_id, error=excluded.error,
			updated_at=excluded.updated_at, finished_at=excluded.finished_at`,
		j.ID, j.Kind, string(j.Status), j.Progress,
		j.EntityID, j.Error,
		encodeTime(j.CreatedAt), encodeTime(j.UpdatedAt),
		encodeOptTime(j.FinishedAt),
	)
	return err
}

func (r *JobRepo) FindByID(ctx context.Context, id string) (*domain.Job, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id,kind,status,progress,entity_id,error,created_at,updated_at,finished_at FROM jobs WHERE id=?`, id)
	j, err := scanJob(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	return j, err
}

func (r *JobRepo) FindAll(ctx context.Context, f domain.JobFilter) ([]domain.Job, error) {
	q := `SELECT id,kind,status,progress,entity_id,error,created_at,updated_at,finished_at FROM jobs WHERE 1=1`
	args := []interface{}{}
	if f.Status != "" {
		q += " AND status=?"
		args = append(args, string(f.Status))
	}
	if f.Kind != "" {
		q += " AND kind=?"
		args = append(args, f.Kind)
	}
	q += " ORDER BY created_at DESC"
	if f.Limit > 0 {
		q += " LIMIT ?"
		args = append(args, f.Limit)
	}
	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var jobs []domain.Job
	for rows.Next() {
		j, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, *j)
	}
	return jobs, rows.Err()
}

func (r *JobRepo) FindActive(ctx context.Context, limit int) ([]domain.Job, error) {
	query := `SELECT id,kind,status,progress,entity_id,error,created_at,updated_at,finished_at FROM jobs WHERE status IN ('pending','scheduled','running','network_busy') AND kind IN ('refresh_upstream','test_connectivity','test_ping','test_throughput','test_group','test_node','recalculate_score','core_reload') ORDER BY created_at ASC`
	args := []any{}
	if limit > 0 {
		query += " LIMIT ?"
		args = append(args, limit)
	}
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var jobs []domain.Job
	for rows.Next() {
		job, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, *job)
	}
	return jobs, rows.Err()
}

func (r *JobRepo) CancelOrphanedActive(ctx context.Context) error {
	now := encodeTime(nowUTC())
	_, err := r.db.ExecContext(ctx, `UPDATE jobs SET status='cancelled',progress=100,error='service_restarted',updated_at=?,finished_at=? WHERE status IN ('pending','scheduled','running','network_busy')`, now, now)
	return err
}

func (r *JobRepo) DeleteInactive(ctx context.Context) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM jobs WHERE status NOT IN ('pending','scheduled','running','network_busy')`)
	return err
}

func (r *JobRepo) DeleteAll(ctx context.Context) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM jobs`)
	return err
}

func (r *JobRepo) UpdateStatus(ctx context.Context, id string, status domain.JobStatus, progress int, errMsg string) error {
	if !validJobStatus(status) {
		return fmt.Errorf("invalid job status %q", status)
	}
	var finishedAt interface{}
	if status == domain.JobDone || status == domain.JobSucceeded || status == domain.JobFailed || status == domain.JobCancelled {
		now := nowUTC()
		finishedAt = encodeTime(now)
	}
	res, err := r.db.ExecContext(ctx,
		`UPDATE jobs SET status=?, progress=?, error=?, updated_at=?, finished_at=? WHERE id=?`,
		string(status), progress, errMsg, encodeTime(nowUTC()), finishedAt, id,
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

func validJobStatus(status domain.JobStatus) bool {
	switch status {
	case domain.JobScheduled, domain.JobPending, domain.JobRunning, domain.JobDone,
		domain.JobSucceeded, domain.JobFailed, domain.JobCancelled, domain.JobNetworkBusy:
		return true
	default:
		return false
	}
}

type jobScanner interface {
	Scan(dest ...interface{}) error
}

func scanJob(row jobScanner) (*domain.Job, error) {
	var j domain.Job
	var status, createdAt, updatedAt string
	var finishedAt sql.NullString
	if err := row.Scan(
		&j.ID, &j.Kind, &status, &j.Progress,
		&j.EntityID, &j.Error,
		&createdAt, &updatedAt, &finishedAt,
	); err != nil {
		return nil, err
	}
	j.Status = domain.JobStatus(status)
	j.CreatedAt = decodeTime(createdAt)
	j.UpdatedAt = decodeTime(updatedAt)
	j.FinishedAt = decodeOptTime(finishedAt)
	return &j, nil
}
