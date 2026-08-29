package persistence

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/teddymail/bagualu/internal/domain"
)

// MeasurementRepo is the SQLite implementation of domain.MeasurementRepository.
type MeasurementRepo struct{ db *sql.DB }

func NewMeasurementRepo(db *sql.DB) *MeasurementRepo { return &MeasurementRepo{db: db} }

func (r *MeasurementRepo) Save(ctx context.Context, m *domain.Measurement) error {
	evidenceJSON, _ := json.Marshal(m.CoreEvidence)
	contextJSON, _ := json.Marshal(map[string]any{
		"proxy_protocol": m.ProxyProtocol, "test_url": m.TestURL, "exit_ip": m.ExitIP,
		"baseline_target": m.BaselineTarget, "speed_source": m.SpeedSource,
		"load_status": m.LoadStatus, "background_upload_bps": m.BackgroundUploadBPS,
		"background_download_bps": m.BackgroundDownloadBPS, "upload_bytes": m.UploadBytes,
		"effective_download_duration_ms": m.EffectiveDownloadDurationMS,
		"wan_download_before":            m.WANDownloadBefore, "wan_download_after": m.WANDownloadAfter,
		"wan_upload_before": m.WANUploadBefore, "wan_upload_after": m.WANUploadAfter,
		"wan_download_capacity_bps": m.WANDownloadCapacityBPS, "wan_upload_capacity_bps": m.WANUploadCapacityBPS,
		"load_threshold": m.LoadThreshold, "load_sample_duration_ms": m.LoadSampleDurationMS,
	})
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO measurements(id,node_id,kind,success,error_code,failure_stage,
			latency_ms,first_byte_ms,speed_bytes_per_sec,bytes,infrastructure,evidence_json,context_json,created_at)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(id) DO NOTHING`,
		m.ID, m.NodeID, m.Kind, boolToInt(m.Success),
		m.ErrorCode, m.FailureStage,
		m.LatencyMS, m.FirstByteMS, m.SpeedBytesPerSec, m.Bytes,
		boolToInt(m.Infrastructure),
		string(evidenceJSON), string(contextJSON), encodeTime(m.CreatedAt),
	)
	return err
}

func (r *MeasurementRepo) FindByNodeID(ctx context.Context, nodeID string, limit int) ([]domain.Measurement, error) {
	q := `SELECT id,node_id,kind,success,error_code,failure_stage,
		latency_ms,first_byte_ms,speed_bytes_per_sec,bytes,infrastructure,evidence_json,context_json,created_at
		FROM measurements WHERE node_id=? ORDER BY created_at DESC`
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
	return scanMeasurements(rows)
}

func (r *MeasurementRepo) FindSince(ctx context.Context, nodeID string, since time.Time) ([]domain.Measurement, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id,node_id,kind,success,error_code,failure_stage,
			latency_ms,first_byte_ms,speed_bytes_per_sec,bytes,infrastructure,evidence_json,context_json,created_at
		FROM measurements WHERE node_id=? AND created_at >= ? ORDER BY created_at ASC`,
		nodeID, encodeTime(since),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanMeasurements(rows)
}

func (r *MeasurementRepo) FindLatestTimesByKind(ctx context.Context, kind string, excludeInfrastructure bool) (map[string]time.Time, error) {
	query := `SELECT node_id, MAX(created_at) FROM measurements WHERE kind=?`
	if excludeInfrastructure {
		query += ` AND infrastructure=0`
	}
	query += ` GROUP BY node_id`
	rows, err := r.db.QueryContext(ctx, query, kind)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make(map[string]time.Time)
	for rows.Next() {
		var nodeID, createdAt string
		if err := rows.Scan(&nodeID, &createdAt); err != nil {
			return nil, err
		}
		result[nodeID] = decodeTime(createdAt)
	}
	return result, rows.Err()
}

func (r *MeasurementRepo) DeleteByNodeID(ctx context.Context, nodeID string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM measurements WHERE node_id=?`, nodeID)
	return err
}

func scanMeasurements(rows *sql.Rows) ([]domain.Measurement, error) {
	var ms []domain.Measurement
	for rows.Next() {
		var m domain.Measurement
		var success, infra int
		var evidenceJSON, contextJSON, createdAt string
		if err := rows.Scan(
			&m.ID, &m.NodeID, &m.Kind, &success,
			&m.ErrorCode, &m.FailureStage,
			&m.LatencyMS, &m.FirstByteMS, &m.SpeedBytesPerSec, &m.Bytes,
			&infra, &evidenceJSON, &contextJSON, &createdAt,
		); err != nil {
			return nil, err
		}
		m.Success = success != 0
		m.Infrastructure = infra != 0
		_ = json.Unmarshal([]byte(evidenceJSON), &m.CoreEvidence)
		decodeMeasurementContext(contextJSON, &m)
		m.CreatedAt = decodeTime(createdAt)
		ms = append(ms, m)
	}
	return ms, rows.Err()
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// FindByID is provided so callers can verify a specific measurement exists.
func (r *MeasurementRepo) FindByID(ctx context.Context, id string) (*domain.Measurement, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id,node_id,kind,success,error_code,failure_stage,
			latency_ms,first_byte_ms,speed_bytes_per_sec,bytes,infrastructure,evidence_json,context_json,created_at
		FROM measurements WHERE id=?`, id)
	var m domain.Measurement
	var success, infra int
	var evidenceJSON, contextJSON, createdAt string
	err := row.Scan(
		&m.ID, &m.NodeID, &m.Kind, &success,
		&m.ErrorCode, &m.FailureStage,
		&m.LatencyMS, &m.FirstByteMS, &m.SpeedBytesPerSec, &m.Bytes,
		&infra, &evidenceJSON, &contextJSON, &createdAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	m.Success = success != 0
	m.Infrastructure = infra != 0
	_ = json.Unmarshal([]byte(evidenceJSON), &m.CoreEvidence)
	decodeMeasurementContext(contextJSON, &m)
	m.CreatedAt = decodeTime(createdAt)
	return &m, nil
}

func decodeMeasurementContext(raw string, measurement *domain.Measurement) {
	var values struct {
		ProxyProtocol               string  `json:"proxy_protocol"`
		TestURL                     string  `json:"test_url"`
		ExitIP                      string  `json:"exit_ip"`
		BaselineTarget              string  `json:"baseline_target"`
		SpeedSource                 string  `json:"speed_source"`
		LoadStatus                  string  `json:"load_status"`
		BackgroundUploadBPS         float64 `json:"background_upload_bps"`
		BackgroundDownloadBPS       float64 `json:"background_download_bps"`
		UploadBytes                 int64   `json:"upload_bytes"`
		EffectiveDownloadDurationMS float64 `json:"effective_download_duration_ms"`
		WANDownloadBefore           int64   `json:"wan_download_before"`
		WANDownloadAfter            int64   `json:"wan_download_after"`
		WANUploadBefore             int64   `json:"wan_upload_before"`
		WANUploadAfter              int64   `json:"wan_upload_after"`
		WANDownloadCapacityBPS      float64 `json:"wan_download_capacity_bps"`
		WANUploadCapacityBPS        float64 `json:"wan_upload_capacity_bps"`
		LoadThreshold               float64 `json:"load_threshold"`
		LoadSampleDurationMS        int64   `json:"load_sample_duration_ms"`
	}
	if json.Unmarshal([]byte(raw), &values) != nil {
		return
	}
	measurement.ProxyProtocol = values.ProxyProtocol
	measurement.TestURL = values.TestURL
	measurement.ExitIP = values.ExitIP
	measurement.BaselineTarget = values.BaselineTarget
	measurement.SpeedSource = values.SpeedSource
	measurement.LoadStatus = values.LoadStatus
	measurement.BackgroundUploadBPS = values.BackgroundUploadBPS
	measurement.BackgroundDownloadBPS = values.BackgroundDownloadBPS
	measurement.UploadBytes = values.UploadBytes
	measurement.EffectiveDownloadDurationMS = values.EffectiveDownloadDurationMS
	measurement.WANDownloadBefore = values.WANDownloadBefore
	measurement.WANDownloadAfter = values.WANDownloadAfter
	measurement.WANUploadBefore = values.WANUploadBefore
	measurement.WANUploadAfter = values.WANUploadAfter
	measurement.WANDownloadCapacityBPS = values.WANDownloadCapacityBPS
	measurement.WANUploadCapacityBPS = values.WANUploadCapacityBPS
	measurement.LoadThreshold = values.LoadThreshold
	measurement.LoadSampleDurationMS = values.LoadSampleDurationMS
}
