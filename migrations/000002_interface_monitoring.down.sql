DROP INDEX IF EXISTS idx_interface_samples_interface_captured_at;
DROP TABLE IF EXISTS interface_samples;
DROP TABLE IF EXISTS interfaces;

INSERT INTO app_metadata (key, value)
VALUES ('bootstrap_phase', 'phase_1')
ON CONFLICT(key) DO UPDATE SET
  value = excluded.value,
  updated_at = CURRENT_TIMESTAMP;