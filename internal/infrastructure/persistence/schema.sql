PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS nodes (
 id TEXT PRIMARY KEY, name TEXT NOT NULL, protocol TEXT NOT NULL, address TEXT NOT NULL,
 port INTEGER NOT NULL, endpoint_ip TEXT NOT NULL DEFAULT '', exit_ip TEXT NOT NULL DEFAULT '',
	country TEXT NOT NULL DEFAULT '', city TEXT NOT NULL DEFAULT '', asn TEXT NOT NULL DEFAULT '', organization TEXT NOT NULL DEFAULT '', region TEXT NOT NULL DEFAULT '',
	geo_source TEXT NOT NULL DEFAULT '', geo_updated_at TEXT, region_changed_at TEXT,
 source_url TEXT NOT NULL DEFAULT '', status TEXT NOT NULL DEFAULT 'active',
 raw_config TEXT NOT NULL DEFAULT '{}', created_at TEXT NOT NULL, updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS measurements (
 id TEXT PRIMARY KEY, node_id TEXT NOT NULL, kind TEXT NOT NULL, success INTEGER NOT NULL,
 error_code TEXT NOT NULL DEFAULT '', failure_stage TEXT NOT NULL DEFAULT '',
 latency_ms REAL NOT NULL DEFAULT 0, first_byte_ms REAL NOT NULL DEFAULT 0,
 speed_bytes_per_sec REAL NOT NULL DEFAULT 0, bytes INTEGER NOT NULL DEFAULT 0,
	infrastructure INTEGER NOT NULL DEFAULT 0, evidence_json TEXT NOT NULL DEFAULT '{}', context_json TEXT NOT NULL DEFAULT '{}',
 created_at TEXT NOT NULL, FOREIGN KEY(node_id) REFERENCES nodes(id)
);
CREATE INDEX IF NOT EXISTS measurements_node_created ON measurements(node_id, created_at);

CREATE TABLE IF NOT EXISTS settings (key TEXT PRIMARY KEY, value TEXT NOT NULL);

-- Upstream subscriptions.
CREATE TABLE IF NOT EXISTS upstreams (
 id TEXT PRIMARY KEY, name TEXT NOT NULL, url TEXT NOT NULL,
 format TEXT NOT NULL DEFAULT 'clash', refresh_interval_sec INTEGER NOT NULL DEFAULT 3600,
 enabled INTEGER NOT NULL DEFAULT 1, notes TEXT NOT NULL DEFAULT '',
 created_at TEXT NOT NULL, updated_at TEXT NOT NULL
);

-- One record per upstream refresh attempt.
CREATE TABLE IF NOT EXISTS refresh_records (
 id TEXT PRIMARY KEY, upstream_id TEXT NOT NULL, success INTEGER NOT NULL DEFAULT 0,
 error TEXT NOT NULL DEFAULT '', node_count INTEGER NOT NULL DEFAULT 0,
 created_at TEXT NOT NULL, FOREIGN KEY(upstream_id) REFERENCES upstreams(id)
);
CREATE INDEX IF NOT EXISTS refresh_records_upstream_created ON refresh_records(upstream_id, created_at);

-- Tracks which upstream(s) each node originated from.
CREATE TABLE IF NOT EXISTS node_sources (
 node_id TEXT NOT NULL, upstream_id TEXT NOT NULL,
 original_name TEXT NOT NULL DEFAULT '', raw_fragment TEXT NOT NULL DEFAULT '',
 created_at TEXT NOT NULL,
 PRIMARY KEY(node_id, upstream_id),
 FOREIGN KEY(node_id) REFERENCES nodes(id),
 FOREIGN KEY(upstream_id) REFERENCES upstreams(id)
);

-- Named groups of nodes used for access control and resource output.
CREATE TABLE IF NOT EXISTS groups (
 id TEXT PRIMARY KEY, name TEXT NOT NULL, description TEXT NOT NULL DEFAULT '',
 min_score REAL NOT NULL DEFAULT 60, one_per_endpoint_ip INTEGER NOT NULL DEFAULT 1,
 created_at TEXT NOT NULL, updated_at TEXT NOT NULL
);

-- Many-to-many: nodes <-> groups.
CREATE TABLE IF NOT EXISTS node_groups (
 node_id TEXT NOT NULL, group_id TEXT NOT NULL,
 PRIMARY KEY(node_id, group_id),
 FOREIGN KEY(node_id) REFERENCES nodes(id),
 FOREIGN KEY(group_id) REFERENCES groups(id)
);
CREATE INDEX IF NOT EXISTS node_groups_group_id ON node_groups(group_id);

-- Background jobs: upstream refresh, node tests, score recalculation.
CREATE TABLE IF NOT EXISTS jobs (
 id TEXT PRIMARY KEY, kind TEXT NOT NULL, status TEXT NOT NULL DEFAULT 'pending',
 progress INTEGER NOT NULL DEFAULT 0, entity_id TEXT NOT NULL DEFAULT '',
 error TEXT NOT NULL DEFAULT '',
 error_code TEXT NOT NULL DEFAULT '', error_detail TEXT NOT NULL DEFAULT '', failure_stage TEXT NOT NULL DEFAULT '',
 created_at TEXT NOT NULL, updated_at TEXT NOT NULL, finished_at TEXT
);
CREATE INDEX IF NOT EXISTS jobs_status ON jobs(status);

-- API keys: only SHA-256 hash is stored, never the plaintext key.
CREATE TABLE IF NOT EXISTS api_keys (
 id TEXT PRIMARY KEY, name TEXT NOT NULL, group_id TEXT NOT NULL,
 key_hash TEXT NOT NULL UNIQUE, prefix TEXT NOT NULL DEFAULT '',
 expires_at TEXT, revoked_at TEXT,
 created_at TEXT NOT NULL, updated_at TEXT NOT NULL
);

-- Subscription links: only SHA-256 hash of the token is stored, never plaintext.
CREATE TABLE IF NOT EXISTS subscription_links (
 id TEXT PRIMARY KEY, name TEXT NOT NULL, group_id TEXT NOT NULL,
 token_hash TEXT NOT NULL UNIQUE, token_prefix TEXT NOT NULL DEFAULT '',
 default_format TEXT NOT NULL DEFAULT 'clash', min_score REAL NOT NULL DEFAULT 60,
 max_nodes INTEGER NOT NULL DEFAULT 0, healthy_only INTEGER NOT NULL DEFAULT 1,
 enabled INTEGER NOT NULL DEFAULT 1,
 expires_at TEXT, last_access_at TEXT,
 allowed_formats TEXT NOT NULL DEFAULT '',
 created_at TEXT NOT NULL, updated_at TEXT NOT NULL
);

-- Immutable point-in-time score snapshots for each node.
CREATE TABLE IF NOT EXISTS score_snapshots (
 id TEXT PRIMARY KEY, node_id TEXT NOT NULL,
 latency REAL NOT NULL DEFAULT 0, speed REAL NOT NULL DEFAULT 0,
 availability REAL NOT NULL DEFAULT 0, overall REAL NOT NULL DEFAULT 0,
 status TEXT NOT NULL DEFAULT 'unrated',
 latency_samples INTEGER NOT NULL DEFAULT 0, speed_samples INTEGER NOT NULL DEFAULT 0,
 availability_samples INTEGER NOT NULL DEFAULT 0, strategy_version INTEGER NOT NULL DEFAULT 1,
 calculated_at TEXT NOT NULL,
 FOREIGN KEY(node_id) REFERENCES nodes(id)
);
CREATE INDEX IF NOT EXISTS score_snapshots_node_calculated ON score_snapshots(node_id, calculated_at);
