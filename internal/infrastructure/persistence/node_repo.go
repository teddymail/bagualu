package persistence

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/teddymail/bagualu/internal/domain"
)

// NodeRepo is the SQLite implementation of domain.NodeRepository.
type NodeRepo struct{ db *sql.DB }

func NewNodeRepo(db *sql.DB) *NodeRepo { return &NodeRepo{db: db} }

func (r *NodeRepo) Save(ctx context.Context, n *domain.Node) error {
	rawCfg := marshalJSON(n.RawConfig)
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO nodes(id,name,protocol,address,port,endpoint_ip,exit_ip,country,city,asn,organization,region,geo_source,geo_updated_at,region_changed_at,source_url,status,raw_config,created_at,updated_at)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET
			name=excluded.name, protocol=excluded.protocol, address=excluded.address,
			port=excluded.port, endpoint_ip=excluded.endpoint_ip, exit_ip=excluded.exit_ip,
			country=excluded.country, city=excluded.city, asn=excluded.asn, organization=excluded.organization, region=excluded.region,
			geo_source=excluded.geo_source, geo_updated_at=excluded.geo_updated_at, region_changed_at=excluded.region_changed_at,
			source_url=excluded.source_url, status=excluded.status,
			raw_config=excluded.raw_config, updated_at=excluded.updated_at`,
		n.ID, n.Name, n.Protocol, n.Address, n.Port,
		n.EndpointIP, n.ExitIP, n.Country, n.City, n.ASN, n.Organization, n.Region, n.GeoSource,
		encodeOptTime(n.GeoUpdatedAt), encodeOptTime(n.RegionChangedAt), n.SourceURL, string(n.Status),
		rawCfg, encodeTime(n.CreatedAt), encodeTime(n.UpdatedAt),
	)
	return err
}

func (r *NodeRepo) FindByID(ctx context.Context, id string) (*domain.Node, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id,name,protocol,address,port,endpoint_ip,exit_ip,country,city,asn,organization,region,geo_source,geo_updated_at,region_changed_at,source_url,status,raw_config,created_at,updated_at
		 FROM nodes WHERE id=?`, id)
	n, err := scanNode(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	return n, err
}

func (r *NodeRepo) FindAll(ctx context.Context, f domain.NodeFilter) ([]domain.Node, error) {
	q := `SELECT id,name,protocol,address,port,endpoint_ip,exit_ip,country,city,asn,organization,region,geo_source,geo_updated_at,region_changed_at,source_url,status,raw_config,created_at,updated_at FROM nodes WHERE 1=1`
	args := []interface{}{}

	if f.Status != "" {
		q += " AND status=?"
		args = append(args, string(f.Status))
	}
	if f.Protocol != "" {
		q += " AND protocol=?"
		args = append(args, f.Protocol)
	}
	if f.Region != "" {
		q += " AND region=?"
		args = append(args, f.Region)
	}
	if f.GroupID != "" {
		q += " AND id IN (SELECT node_id FROM node_groups WHERE group_id=?)"
		args = append(args, f.GroupID)
	}
	if search := strings.TrimSpace(f.Search); search != "" {
		q += " AND (name LIKE ? OR address LIKE ? OR endpoint_ip LIKE ? OR source_url LIKE ?)"
		pattern := "%" + search + "%"
		args = append(args, pattern, pattern, pattern, pattern)
	}
	orderBy := "created_at DESC"
	switch f.Sort {
	case "name":
		orderBy = "name COLLATE NOCASE ASC, id ASC"
	case "address":
		orderBy = "address COLLATE NOCASE ASC, id ASC"
	case "updated":
		orderBy = "updated_at DESC, id ASC"
	case "status":
		orderBy = "status ASC, updated_at DESC, id ASC"
	case "created", "":
	default:
		return nil, fmt.Errorf("unsupported node sort %q", f.Sort)
	}
	q += " ORDER BY " + orderBy
	if f.Limit > 0 {
		q += " LIMIT ? OFFSET ?"
		args = append(args, f.Limit, f.Offset)
	}

	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanNodes(rows)
}

func (r *NodeRepo) Delete(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM nodes WHERE id=?`, id)
	return err
}

func (r *NodeRepo) UpdateStatus(ctx context.Context, id string, status domain.NodeStatus) error {
	res, err := r.db.ExecContext(ctx, `UPDATE nodes SET status=? WHERE id=?`, string(status), id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *NodeRepo) SaveNodeSource(ctx context.Context, src domain.NodeSource) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO node_sources(node_id,upstream_id,original_name,raw_fragment,created_at)
		VALUES(?,?,?,?,?)
		ON CONFLICT(node_id,upstream_id) DO UPDATE SET
			original_name=excluded.original_name, raw_fragment=excluded.raw_fragment`,
		src.NodeID, src.UpstreamID, src.OriginalName, src.RawFragment, encodeTime(src.CreatedAt),
	)
	return err
}

func (r *NodeRepo) FindNodeIDsByUpstream(ctx context.Context, upstreamID string) ([]string, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT node_id FROM node_sources WHERE upstream_id=?`, upstreamID)
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

func (r *NodeRepo) DeleteNodeSource(ctx context.Context, nodeID, upstreamID string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM node_sources WHERE node_id=? AND upstream_id=?`, nodeID, upstreamID)
	return err
}

func (r *NodeRepo) FindNodeSources(ctx context.Context, nodeID string) ([]domain.NodeSource, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT node_id,upstream_id,original_name,raw_fragment,created_at FROM node_sources WHERE node_id=?`,
		nodeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var srcs []domain.NodeSource
	for rows.Next() {
		var s domain.NodeSource
		var createdAt string
		if err := rows.Scan(&s.NodeID, &s.UpstreamID, &s.OriginalName, &s.RawFragment, &createdAt); err != nil {
			return nil, err
		}
		s.CreatedAt = decodeTime(createdAt)
		srcs = append(srcs, s)
	}
	return srcs, rows.Err()
}

// scanNode scans one node row.
type nodeScanner interface {
	Scan(dest ...interface{}) error
}

func scanNode(row nodeScanner) (*domain.Node, error) {
	var n domain.Node
	var status, rawCfg, createdAt, updatedAt string
	var geoUpdatedAt, regionChangedAt sql.NullString
	err := row.Scan(
		&n.ID, &n.Name, &n.Protocol, &n.Address, &n.Port,
		&n.EndpointIP, &n.ExitIP, &n.Country, &n.City, &n.ASN, &n.Organization, &n.Region, &n.GeoSource,
		&geoUpdatedAt, &regionChangedAt, &n.SourceURL,
		&status, &rawCfg, &createdAt, &updatedAt,
	)
	if err != nil {
		return nil, err
	}
	n.Status = domain.NodeStatus(status)
	n.RawConfig = map[string]any{}
	unmarshalJSON(rawCfg, &n.RawConfig)
	n.CreatedAt = decodeTime(createdAt)
	n.UpdatedAt = decodeTime(updatedAt)
	if geoUpdatedAt.Valid {
		value := decodeTime(geoUpdatedAt.String)
		n.GeoUpdatedAt = &value
	}
	if regionChangedAt.Valid {
		value := decodeTime(regionChangedAt.String)
		n.RegionChangedAt = &value
	}
	return &n, nil
}

func scanNodes(rows *sql.Rows) ([]domain.Node, error) {
	var nodes []domain.Node
	for rows.Next() {
		n, err := scanNode(rows)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, *n)
	}
	return nodes, rows.Err()
}
