CREATE TABLE IF NOT EXISTS interfaces (
  id INTEGER PRIMARY KEY,
  name TEXT NOT NULL UNIQUE,
  display_name TEXT NOT NULL,
  is_active INTEGER NOT NULL DEFAULT 1,
  is_loopback INTEGER NOT NULL DEFAULT 0,
  last_seen_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS interface_samples (
  id INTEGER PRIMARY KEY,
  interface_id INTEGER NOT NULL REFERENCES interfaces(id) ON DELETE CASCADE,
  captured_at TEXT NOT NULL,
  rx_bytes_total INTEGER NOT NULL,
  tx_bytes_total INTEGER NOT NULL,
  rx_bps REAL NOT NULL,
  tx_bps REAL NOT NULL,
  UNIQUE(interface_id, captured_at)
);

CREATE INDEX IF NOT EXISTS idx_interface_samples_interface_captured_at
  ON interface_samples (interface_id, captured_at DESC);

INSERT INTO app_metadata (key, value)
VALUES ('bootstrap_phase', 'phase_2')
ON CONFLICT(key) DO UPDATE SET
  value = excluded.value,
  updated_at = CURRENT_TIMESTAMP;