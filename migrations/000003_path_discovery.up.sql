CREATE TABLE IF NOT EXISTS targets (
  id INTEGER PRIMARY KEY,
  name TEXT NOT NULL,
  host TEXT NOT NULL UNIQUE,
  enabled INTEGER NOT NULL DEFAULT 1,
  probe_interval_seconds INTEGER NOT NULL DEFAULT 45,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS trace_runs (
  id INTEGER PRIMARY KEY,
  target_id INTEGER NOT NULL REFERENCES targets(id) ON DELETE CASCADE,
  started_at TEXT NOT NULL,
  completed_at TEXT NOT NULL,
  hop_count INTEGER NOT NULL,
  status TEXT NOT NULL,
  route_fingerprint TEXT NOT NULL DEFAULT '',
  route_changed INTEGER NOT NULL DEFAULT 0,
  degraded_hop_count INTEGER NOT NULL DEFAULT 0,
  error_message TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_trace_runs_target_started_at
  ON trace_runs (target_id, started_at DESC);

CREATE TABLE IF NOT EXISTS trace_hops (
  id INTEGER PRIMARY KEY,
  trace_run_id INTEGER NOT NULL REFERENCES trace_runs(id) ON DELETE CASCADE,
  hop_index INTEGER NOT NULL,
  address TEXT NOT NULL DEFAULT '',
  hostname TEXT NOT NULL DEFAULT '',
  avg_rtt_ms REAL NOT NULL DEFAULT 0,
  jitter_ms REAL NOT NULL DEFAULT 0,
  loss_pct REAL NOT NULL DEFAULT 100,
  is_timeout INTEGER NOT NULL DEFAULT 0,
  UNIQUE(trace_run_id, hop_index)
);

CREATE INDEX IF NOT EXISTS idx_trace_hops_run_hop_index
  ON trace_hops (trace_run_id, hop_index ASC);

INSERT INTO app_metadata (key, value)
VALUES ('bootstrap_phase', 'phase_3')
ON CONFLICT(key) DO UPDATE SET
  value = excluded.value,
  updated_at = CURRENT_TIMESTAMP;