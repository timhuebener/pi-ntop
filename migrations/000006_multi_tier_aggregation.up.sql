-- Interface daily rollups (from 1-minute rollups)
CREATE TABLE IF NOT EXISTS interface_samples_1d (
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

CREATE INDEX IF NOT EXISTS idx_interface_samples_1d_interface_window_start
  ON interface_samples_1d (interface_id, window_start DESC);

-- Interface weekly rollups (from daily rollups)
CREATE TABLE IF NOT EXISTS interface_samples_1w (
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

CREATE INDEX IF NOT EXISTS idx_interface_samples_1w_interface_window_start
  ON interface_samples_1w (interface_id, window_start DESC);

-- Interface monthly rollups (from weekly rollups)
CREATE TABLE IF NOT EXISTS interface_samples_1mo (
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

CREATE INDEX IF NOT EXISTS idx_interface_samples_1mo_interface_window_start
  ON interface_samples_1mo (interface_id, window_start DESC);

-- Speed test daily rollups
CREATE TABLE IF NOT EXISTS speed_tests_1d (
  id INTEGER PRIMARY KEY,
  target_id INTEGER NOT NULL REFERENCES speed_test_targets(id) ON DELETE CASCADE,
  window_start TEXT NOT NULL,
  test_count INTEGER NOT NULL DEFAULT 0,
  avg_download_bps REAL NOT NULL DEFAULT 0,
  avg_upload_bps REAL NOT NULL DEFAULT 0,
  avg_latency_ms REAL NOT NULL DEFAULT 0,
  peak_download_bps REAL NOT NULL DEFAULT 0,
  peak_upload_bps REAL NOT NULL DEFAULT 0,
  fail_count INTEGER NOT NULL DEFAULT 0,
  UNIQUE(target_id, window_start)
);

CREATE INDEX IF NOT EXISTS idx_speed_tests_1d_target_window_start
  ON speed_tests_1d (target_id, window_start DESC);

-- Speed test weekly rollups
CREATE TABLE IF NOT EXISTS speed_tests_1w (
  id INTEGER PRIMARY KEY,
  target_id INTEGER NOT NULL REFERENCES speed_test_targets(id) ON DELETE CASCADE,
  window_start TEXT NOT NULL,
  test_count INTEGER NOT NULL DEFAULT 0,
  avg_download_bps REAL NOT NULL DEFAULT 0,
  avg_upload_bps REAL NOT NULL DEFAULT 0,
  avg_latency_ms REAL NOT NULL DEFAULT 0,
  peak_download_bps REAL NOT NULL DEFAULT 0,
  peak_upload_bps REAL NOT NULL DEFAULT 0,
  fail_count INTEGER NOT NULL DEFAULT 0,
  UNIQUE(target_id, window_start)
);

CREATE INDEX IF NOT EXISTS idx_speed_tests_1w_target_window_start
  ON speed_tests_1w (target_id, window_start DESC);

-- Speed test monthly rollups
CREATE TABLE IF NOT EXISTS speed_tests_1mo (
  id INTEGER PRIMARY KEY,
  target_id INTEGER NOT NULL REFERENCES speed_test_targets(id) ON DELETE CASCADE,
  window_start TEXT NOT NULL,
  test_count INTEGER NOT NULL DEFAULT 0,
  avg_download_bps REAL NOT NULL DEFAULT 0,
  avg_upload_bps REAL NOT NULL DEFAULT 0,
  avg_latency_ms REAL NOT NULL DEFAULT 0,
  peak_download_bps REAL NOT NULL DEFAULT 0,
  peak_upload_bps REAL NOT NULL DEFAULT 0,
  fail_count INTEGER NOT NULL DEFAULT 0,
  UNIQUE(target_id, window_start)
);

CREATE INDEX IF NOT EXISTS idx_speed_tests_1mo_target_window_start
  ON speed_tests_1mo (target_id, window_start DESC);

-- Trace run daily rollups
CREATE TABLE IF NOT EXISTS trace_runs_1d (
  id INTEGER PRIMARY KEY,
  target_id INTEGER NOT NULL REFERENCES targets(id) ON DELETE CASCADE,
  window_start TEXT NOT NULL,
  run_count INTEGER NOT NULL DEFAULT 0,
  avg_hop_count REAL NOT NULL DEFAULT 0,
  avg_degraded_hop_count REAL NOT NULL DEFAULT 0,
  fail_count INTEGER NOT NULL DEFAULT 0,
  route_change_count INTEGER NOT NULL DEFAULT 0,
  UNIQUE(target_id, window_start)
);

CREATE INDEX IF NOT EXISTS idx_trace_runs_1d_target_window_start
  ON trace_runs_1d (target_id, window_start DESC);

-- Trace run weekly rollups
CREATE TABLE IF NOT EXISTS trace_runs_1w (
  id INTEGER PRIMARY KEY,
  target_id INTEGER NOT NULL REFERENCES targets(id) ON DELETE CASCADE,
  window_start TEXT NOT NULL,
  run_count INTEGER NOT NULL DEFAULT 0,
  avg_hop_count REAL NOT NULL DEFAULT 0,
  avg_degraded_hop_count REAL NOT NULL DEFAULT 0,
  fail_count INTEGER NOT NULL DEFAULT 0,
  route_change_count INTEGER NOT NULL DEFAULT 0,
  UNIQUE(target_id, window_start)
);

CREATE INDEX IF NOT EXISTS idx_trace_runs_1w_target_window_start
  ON trace_runs_1w (target_id, window_start DESC);

-- Trace run monthly rollups
CREATE TABLE IF NOT EXISTS trace_runs_1mo (
  id INTEGER PRIMARY KEY,
  target_id INTEGER NOT NULL REFERENCES targets(id) ON DELETE CASCADE,
  window_start TEXT NOT NULL,
  run_count INTEGER NOT NULL DEFAULT 0,
  avg_hop_count REAL NOT NULL DEFAULT 0,
  avg_degraded_hop_count REAL NOT NULL DEFAULT 0,
  fail_count INTEGER NOT NULL DEFAULT 0,
  route_change_count INTEGER NOT NULL DEFAULT 0,
  UNIQUE(target_id, window_start)
);

CREATE INDEX IF NOT EXISTS idx_trace_runs_1mo_target_window_start
  ON trace_runs_1mo (target_id, window_start DESC);

UPDATE app_metadata
SET value = 'phase_6', updated_at = CURRENT_TIMESTAMP
WHERE key = 'bootstrap_phase';
