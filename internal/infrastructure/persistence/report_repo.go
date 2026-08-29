package persistence

import (
	"context"
	"database/sql"
	"time"
)

type NodeReport struct {
	NodeID, Kind                 string
	Count, SuccessCount          int
	Bytes                        int64
	AverageLatency, AverageSpeed float64
	LastAt                       time.Time
}

type ReportRepo struct{ db *sql.DB }

func NewReportRepo(db *sql.DB) *ReportRepo { return &ReportRepo{db: db} }

func (r *ReportRepo) NodeReports(ctx context.Context, since time.Time, nodeID, kind string) ([]NodeReport, error) {
	query := `SELECT node_id,kind,COUNT(*),SUM(success),SUM(bytes),AVG(NULLIF(latency_ms,0)),AVG(NULLIF(speed_bytes_per_sec,0)),MAX(created_at)
		FROM measurements WHERE created_at >= ?`
	args := []any{encodeTime(since)}
	if nodeID != "" {
		query += " AND node_id=?"
		args = append(args, nodeID)
	}
	if kind != "" {
		query += " AND kind=?"
		args = append(args, kind)
	}
	query += " GROUP BY node_id,kind ORDER BY MAX(created_at) DESC"
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var reports []NodeReport
	for rows.Next() {
		var report NodeReport
		var success int
		var averageLatency, averageSpeed sql.NullFloat64
		var lastAt string
		if err := rows.Scan(&report.NodeID, &report.Kind, &report.Count, &success, &report.Bytes, &averageLatency, &averageSpeed, &lastAt); err != nil {
			return nil, err
		}
		report.SuccessCount = success
		report.AverageLatency = averageLatency.Float64
		report.AverageSpeed = averageSpeed.Float64
		report.LastAt = decodeTime(lastAt)
		reports = append(reports, report)
	}
	return reports, rows.Err()
}

func (r *ReportRepo) Summary(ctx context.Context, since time.Time) (map[string]any, error) {
	var measurements, successes, infrastructure int
	var bytes int64
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*),COALESCE(SUM(success),0),COALESCE(SUM(bytes),0),COALESCE(SUM(infrastructure),0) FROM measurements WHERE created_at >= ?`, encodeTime(since)).Scan(&measurements, &successes, &bytes, &infrastructure)
	if err != nil {
		return nil, err
	}
	return map[string]any{"measurement_count": measurements, "success_count": successes, "bytes": bytes, "infrastructure_count": infrastructure}, nil
}

func (r *ReportRepo) Cleanup(ctx context.Context, measurementBefore, refreshBefore, snapshotBefore, jobBefore time.Time) error {
	return r.CleanupWithRetention(ctx, measurementBefore, measurementBefore, refreshBefore, snapshotBefore, jobBefore)
}

func (r *ReportRepo) CleanupWithRetention(ctx context.Context, measurementBefore, subscriptionMeasurementBefore, refreshBefore, snapshotBefore, jobBefore time.Time) error {
	for query, args := range map[string][]any{
		`DELETE FROM measurements WHERE created_at < ? OR (created_at < ? AND node_id IN (SELECT node_id FROM node_sources))`: {encodeTime(measurementBefore), encodeTime(subscriptionMeasurementBefore)},
		`DELETE FROM refresh_records WHERE created_at < ?`:                                                                    {encodeTime(refreshBefore)},
		`DELETE FROM score_snapshots WHERE calculated_at < ?`:                                                                 {encodeTime(snapshotBefore)},
		`DELETE FROM jobs WHERE updated_at < ? AND status IN ('done','succeeded','failed','cancelled')`:                       {encodeTime(jobBefore)},
	} {
		if _, err := r.db.ExecContext(ctx, query, args...); err != nil {
			return err
		}
	}
	return nil
}
