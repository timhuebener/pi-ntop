CREATE TABLE IF NOT EXISTS speed_test_targets (
  id INTEGER PRIMARY KEY,
  name TEXT NOT NULL,
  download_url TEXT NOT NULL UNIQUE,
  upload_url TEXT NOT NULL DEFAULT '',
  enabled INTEGER NOT NULL DEFAULT 1,
  interval_seconds INTEGER NOT NULL DEFAULT 180,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS speed_tests (
  id INTEGER PRIMARY KEY,
  target_id INTEGER NOT NULL REFERENCES speed_test_targets(id) ON DELETE CASCADE,
  started_at TEXT NOT NULL,
  completed_at TEXT NOT NULL,
  download_bps REAL NOT NULL DEFAULT 0,
  upload_bps REAL NOT NULL DEFAULT 0,
  latency_ms REAL NOT NULL DEFAULT 0,
  download_bytes INTEGER NOT NULL DEFAULT 0,
  upload_bytes INTEGER NOT NULL DEFAULT 0,
  status TEXT NOT NULL,
  error_message TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_speed_test_targets_enabled_name
  ON speed_test_targets (enabled, name ASC);

CREATE INDEX IF NOT EXISTS idx_speed_tests_target_started_at
  ON speed_tests (target_id, started_at DESC);

INSERT INTO app_metadata (key, value)
VALUES ('bootstrap_phase', 'phase_4')
ON CONFLICT(key) DO UPDATE SET
  value = excluded.value,
  updated_at = CURRENT_TIMESTAMP;