CREATE TABLE IF NOT EXISTS alerts (
  id INTEGER PRIMARY KEY,
  dedupe_key TEXT NOT NULL UNIQUE,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  last_seen_at TEXT NOT NULL,
  resolved_at TEXT NOT NULL DEFAULT '',
  severity TEXT NOT NULL,
  source_type TEXT NOT NULL,
  source_id INTEGER NOT NULL DEFAULT 0,
  source_name TEXT NOT NULL DEFAULT '',
  metric_key TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'active',
  threshold_value REAL NOT NULL DEFAULT 0,
  current_value REAL NOT NULL DEFAULT 0,
  message TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_alerts_status_updated_at
  ON alerts (status, updated_at DESC);

CREATE INDEX IF NOT EXISTS idx_alerts_source_status
  ON alerts (source_type, source_id, status);

CREATE TABLE IF NOT EXISTS interface_samples_1m (
  id INTEGER PRIMARY KEY,
  interface_id INTEGER NOT NULL REFERENCES interfaces(id) ON DELETE CASCADE,
  window_start TEXT NOT NULL,
  sample_count INTEGER NOT NULL DEFAULT 0,
  avg_rx_bps REAL NOT NULL DEFAULT 0,
  avg_tx_bps REAL NOT NULL DEFAULT 0,
  peak_rx_bps REAL NOT NULL DEFAULT 0,
  peak_tx_bps REAL NOT NULL DEFAULT 0,
  rx_bytes_total INTEGER NOT NULL DEFAULT 0,
  tx_bytes_total INTEGER NOT NULL DEFAULT 0,
  UNIQUE(interface_id, window_start)
);

CREATE INDEX IF NOT EXISTS idx_interface_samples_1m_interface_window_start
  ON interface_samples_1m (interface_id, window_start DESC);

INSERT INTO app_metadata (key, value)
VALUES ('bootstrap_phase', 'phase_5')
ON CONFLICT(key) DO UPDATE SET
  value = excluded.value,
  updated_at = CURRENT_TIMESTAMP;